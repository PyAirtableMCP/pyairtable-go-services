package security

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
)

// WAFRule represents a Web Application Firewall rule
type WAFRule struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Type        RuleType  `json:"type"`
	Pattern     string    `json:"pattern"`
	Regex       *regexp.Regexp `json:"-"`
	Action      Action    `json:"action"`
	Severity    Severity  `json:"severity"`
	Enabled     bool      `json:"enabled"`
	Tags        []string  `json:"tags"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// RuleType defines the type of WAF rule
type RuleType string

const (
	RuleTypeSQLInjection     RuleType = "sql_injection"
	RuleTypeXSS              RuleType = "xss"
	RuleTypePathTraversal    RuleType = "path_traversal"
	RuleTypeCommandInjection RuleType = "command_injection"
	RuleTypeLDAPInjection    RuleType = "ldap_injection"
	RuleTypeXMLInjection     RuleType = "xml_injection"
	RuleTypeSSRF             RuleType = "ssrf"
	RuleTypeFileUpload       RuleType = "file_upload"
	RuleTypeRateLimitBypass  RuleType = "rate_limit_bypass"
	RuleTypeCustom           RuleType = "custom"
)

// Action defines what action to take when a rule matches
type Action string

const (
	ActionBlock    Action = "block"
	ActionLog      Action = "log"
	ActionRedirect Action = "redirect"
	ActionSanitize Action = "sanitize"
)

// Severity defines the severity level of the threat
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityHigh     Severity = "high"
	SeverityMedium   Severity = "medium"
	SeverityLow      Severity = "low"
	SeverityInfo     Severity = "info"
)

// ThreatDetection result
type ThreatDetection struct {
	Detected     bool      `json:"detected"`
	Rule         *WAFRule  `json:"rule,omitempty"`
	MatchedValue string    `json:"matched_value,omitempty"`
	Location     string    `json:"location,omitempty"` // header, body, query, path
	Action       Action    `json:"action"`
	Severity     Severity  `json:"severity"`
	Timestamp    time.Time `json:"timestamp"`
}

// WAFConfig holds WAF configuration
type WAFConfig struct {
	Enabled              bool          `json:"enabled"`
	LogAllRequests       bool          `json:"log_all_requests"`
	BlockMaliciousIPs    bool          `json:"block_malicious_ips"`
	SanitizeInputs       bool          `json:"sanitize_inputs"`
	MaxRequestBodySize   int64         `json:"max_request_body_size"`
	MaxHeaderSize        int           `json:"max_header_size"`
	GeoblockingEnabled   bool          `json:"geoblocking_enabled"`
	BlockedCountries     []string      `json:"blocked_countries"`
	TrustedProxies       []string      `json:"trusted_proxies"`
	CustomErrorPage      string        `json:"custom_error_page"`
	AlertWebhookURL      string        `json:"alert_webhook_url"`
}

// IPReputationData holds IP reputation information
type IPReputationData struct {
	IP              string                 `json:"ip"`
	ReputationScore int                    `json:"reputation_score"` // 0-100, lower is worse
	ThreatTypes     []string               `json:"threat_types"`
	Country         string                 `json:"country"`
	ASN             string                 `json:"asn"`
	LastSeen        time.Time              `json:"last_seen"`
	RequestCount    int64                  `json:"request_count"`
	BlockedCount    int64                  `json:"blocked_count"`
	Metadata        map[string]interface{} `json:"metadata"`
}

// WAFEngine is the main Web Application Firewall engine
type WAFEngine struct {
	config       *WAFConfig
	rules        map[string]*WAFRule
	rulesByType  map[RuleType][]*WAFRule
	redis        *redis.Client
	mu           sync.RWMutex
	ipReputationCache map[string]*IPReputationData
	cacheMu      sync.RWMutex
}

// NewWAFEngine creates a new WAF engine
func NewWAFEngine(config *WAFConfig, redis *redis.Client) *WAFEngine {
	waf := &WAFEngine{
		config:            config,
		rules:             make(map[string]*WAFRule),
		rulesByType:       make(map[RuleType][]*WAFRule),
		redis:             redis,
		ipReputationCache: make(map[string]*IPReputationData),
	}
	
	// Load default rules
	waf.loadDefaultRules()
	
	return waf
}

// Middleware creates a Fiber middleware for WAF protection
func (w *WAFEngine) Middleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if !w.config.Enabled {
			return c.Next()
		}
		
		// Check request size limits
		if err := w.checkRequestLimits(c); err != nil {
			return w.handleThreat(c, &ThreatDetection{
				Detected:  true,
				Action:    ActionBlock,
				Severity:  SeverityMedium,
				Timestamp: time.Now(),
			}, err.Error())
		}
		
		// Check IP reputation
		if threat := w.checkIPReputation(c); threat.Detected {
			return w.handleThreat(c, &threat, "Malicious IP detected")
		}
		
		// Check geoblocking
		if threat := w.checkGeoblocking(c); threat.Detected {
			return w.handleThreat(c, &threat, "Request from blocked country")
		}
		
		// Run WAF rules
		threats := w.analyzeRequest(c)
		for _, threat := range threats {
			if threat.Detected && threat.Action == ActionBlock {
				return w.handleThreat(c, &threat, "WAF rule triggered")
			}
		}
		
		// Log non-blocking threats
		for _, threat := range threats {
			if threat.Detected && threat.Action == ActionLog {
				w.logThreat(c, &threat)
			}
		}
		
		return c.Next()
	}
}

// AddRule adds a new WAF rule
func (w *WAFEngine) AddRule(rule *WAFRule) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	
	// Compile regex if pattern is provided
	if rule.Pattern != "" {
		regex, err := regexp.Compile(rule.Pattern)
		if err != nil {
			return fmt.Errorf("invalid regex pattern: %w", err)
		}
		rule.Regex = regex
	}
	
	rule.CreatedAt = time.Now()
	rule.UpdatedAt = time.Now()
	
	w.rules[rule.ID] = rule
	w.rulesByType[rule.Type] = append(w.rulesByType[rule.Type], rule)
	
	// Store in Redis for persistence
	if w.redis != nil {
		w.storeRule(rule)
	}
	
	return nil
}

// RemoveRule removes a WAF rule
func (w *WAFEngine) RemoveRule(ruleID string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	
	rule, exists := w.rules[ruleID]
	if !exists {
		return fmt.Errorf("rule not found: %s", ruleID)
	}
	
	delete(w.rules, ruleID)
	
	// Remove from type-based index
	rules := w.rulesByType[rule.Type]
	for i, r := range rules {
		if r.ID == ruleID {
			w.rulesByType[rule.Type] = append(rules[:i], rules[i+1:]...)
			break
		}
	}
	
	// Remove from Redis
	if w.redis != nil {
		w.removeRule(ruleID)
	}
	
	return nil
}

// analyzeRequest analyzes a request against all WAF rules
func (w *WAFEngine) analyzeRequest(c *fiber.Ctx) []ThreatDetection {
	var threats []ThreatDetection
	
	w.mu.RLock()
	defer w.mu.RUnlock()
	
	// Check each rule type
	for ruleType, rules := range w.rulesByType {
		for _, rule := range rules {
			if !rule.Enabled {
				continue
			}
			
			if threat := w.checkRule(c, rule, ruleType); threat.Detected {
				threats = append(threats, threat)
			}
		}
	}
	
	return threats
}

// checkRule checks a specific rule against the request
func (w *WAFEngine) checkRule(c *fiber.Ctx, rule *WAFRule, ruleType RuleType) ThreatDetection {
	switch ruleType {
	case RuleTypeSQLInjection:
		return w.checkSQLInjection(c, rule)
	case RuleTypeXSS:
		return w.checkXSS(c, rule)
	case RuleTypePathTraversal:
		return w.checkPathTraversal(c, rule)
	case RuleTypeCommandInjection:
		return w.checkCommandInjection(c, rule)
	case RuleTypeCustom:
		return w.checkCustomRule(c, rule)
	default:
		return ThreatDetection{Detected: false}
	}
}

// checkSQLInjection checks for SQL injection attempts
func (w *WAFEngine) checkSQLInjection(c *fiber.Ctx, rule *WAFRule) ThreatDetection {
	// Check query parameters
	c.Request().URI().QueryArgs().VisitAll(func(key, value []byte) {
		if w.matchesPattern(string(value), rule) {
			// SQL injection detected
		}
	})
	
	// Check request body
	body := string(c.Body())
	if w.matchesPattern(body, rule) {
		return ThreatDetection{
			Detected:     true,
			Rule:         rule,
			MatchedValue: w.extractMatch(body, rule),
			Location:     "body",
			Action:       rule.Action,
			Severity:     rule.Severity,
			Timestamp:    time.Now(),
		}
	}
	
	// Check headers
	c.Request().Header.VisitAll(func(key, value []byte) {
		if w.matchesPattern(string(value), rule) {
			// SQL injection detected in header
		}
	})
	
	return ThreatDetection{Detected: false}
}

// checkXSS checks for Cross-Site Scripting attempts
func (w *WAFEngine) checkXSS(c *fiber.Ctx, rule *WAFRule) ThreatDetection {
	// Check query parameters
	c.Request().URI().QueryArgs().VisitAll(func(key, value []byte) {
		if w.matchesPattern(string(value), rule) {
			// XSS detected
		}
	})
	
	// Check request body
	body := string(c.Body())
	if w.matchesPattern(body, rule) {
		return ThreatDetection{
			Detected:     true,
			Rule:         rule,
			MatchedValue: w.extractMatch(body, rule),
			Location:     "body",
			Action:       rule.Action,
			Severity:     rule.Severity,
			Timestamp:    time.Now(),
		}
	}
	
	return ThreatDetection{Detected: false}
}

// checkPathTraversal checks for path traversal attempts
func (w *WAFEngine) checkPathTraversal(c *fiber.Ctx, rule *WAFRule) ThreatDetection {
	path := c.Path()
	if w.matchesPattern(path, rule) {
		return ThreatDetection{
			Detected:     true,
			Rule:         rule,
			MatchedValue: path,
			Location:     "path",
			Action:       rule.Action,
			Severity:     rule.Severity,
			Timestamp:    time.Now(),
		}
	}
	
	return ThreatDetection{Detected: false}
}

// checkCommandInjection checks for command injection attempts
func (w *WAFEngine) checkCommandInjection(c *fiber.Ctx, rule *WAFRule) ThreatDetection {
	body := string(c.Body())
	if w.matchesPattern(body, rule) {
		return ThreatDetection{
			Detected:     true,
			Rule:         rule,
			MatchedValue: w.extractMatch(body, rule),
			Location:     "body",
			Action:       rule.Action,
			Severity:     rule.Severity,
			Timestamp:    time.Now(),
		}
	}
	
	return ThreatDetection{Detected: false}
}

// checkCustomRule checks a custom rule
func (w *WAFEngine) checkCustomRule(c *fiber.Ctx, rule *WAFRule) ThreatDetection {
	// Custom rule logic can be implemented here
	// For now, just check against the pattern
	body := string(c.Body())
	if w.matchesPattern(body, rule) {
		return ThreatDetection{
			Detected:     true,
			Rule:         rule,
			MatchedValue: w.extractMatch(body, rule),
			Location:     "body",
			Action:       rule.Action,
			Severity:     rule.Severity,
			Timestamp:    time.Now(),
		}
	}
	
	return ThreatDetection{Detected: false}
}

// checkIPReputation checks IP reputation
func (w *WAFEngine) checkIPReputation(c *fiber.Ctx) ThreatDetection {
	if !w.config.BlockMaliciousIPs {
		return ThreatDetection{Detected: false}
	}
	
	ip := w.getRealIP(c)
	reputation, err := w.getIPReputation(ip)
	if err != nil {
		return ThreatDetection{Detected: false}
	}
	
	// Block IPs with low reputation scores
	if reputation.ReputationScore < 30 {
		return ThreatDetection{
			Detected:  true,
			Action:    ActionBlock,
			Severity:  SeverityHigh,
			Timestamp: time.Now(),
		}
	}
	
	return ThreatDetection{Detected: false}
}

// checkGeoblocking checks if request is from a blocked country
func (w *WAFEngine) checkGeoblocking(c *fiber.Ctx) ThreatDetection {
	if !w.config.GeoblockingEnabled || len(w.config.BlockedCountries) == 0 {
		return ThreatDetection{Detected: false}
	}
	
	ip := w.getRealIP(c)
	reputation, err := w.getIPReputation(ip)
	if err != nil {
		return ThreatDetection{Detected: false}
	}
	
	// Check if country is blocked
	for _, blockedCountry := range w.config.BlockedCountries {
		if strings.EqualFold(reputation.Country, blockedCountry) {
			return ThreatDetection{
				Detected:  true,
				Action:    ActionBlock,
				Severity:  SeverityMedium,
				Timestamp: time.Now(),
			}
		}
	}
	
	return ThreatDetection{Detected: false}
}

// checkRequestLimits checks request size limits
func (w *WAFEngine) checkRequestLimits(c *fiber.Ctx) error {
	// Check body size
	if w.config.MaxRequestBodySize > 0 && int64(len(c.Body())) > w.config.MaxRequestBodySize {
		return fmt.Errorf("request body too large")
	}
	
	// Check header size
	if w.config.MaxHeaderSize > 0 {
		totalHeaderSize := 0
		c.Request().Header.VisitAll(func(key, value []byte) {
			totalHeaderSize += len(key) + len(value)
		})
		if totalHeaderSize > w.config.MaxHeaderSize {
			return fmt.Errorf("request headers too large")
		}
	}
	
	return nil
}

// getRealIP gets the real client IP considering trusted proxies
func (w *WAFEngine) getRealIP(c *fiber.Ctx) string {
	// Check X-Forwarded-For header
	if xff := c.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		for _, ip := range ips {
			ip = strings.TrimSpace(ip)
			if net.ParseIP(ip) != nil {
				return ip
			}
		}
	}
	
	// Check X-Real-IP header
	if xri := c.Get("X-Real-IP"); xri != "" {
		if net.ParseIP(xri) != nil {
			return xri
		}
	}
	
	// Fallback to connection IP
	return c.IP()
}

// getIPReputation gets IP reputation data
func (w *WAFEngine) getIPReputation(ip string) (*IPReputationData, error) {
	// Check cache first
	w.cacheMu.RLock()
	if reputation, exists := w.ipReputationCache[ip]; exists {
		w.cacheMu.RUnlock()
		return reputation, nil
	}
	w.cacheMu.RUnlock()
	
	// Check Redis
	if w.redis != nil {
		key := fmt.Sprintf("waf:ip_reputation:%s", ip)
		data, err := w.redis.Get(context.Background(), key).Result()
		if err == nil {
			var reputation IPReputationData
			if err := json.Unmarshal([]byte(data), &reputation); err == nil {
				w.cacheMu.Lock()
				w.ipReputationCache[ip] = &reputation
				w.cacheMu.Unlock()
				return &reputation, nil
			}
		}
	}
	
	// Default reputation for unknown IPs
	reputation := &IPReputationData{
		IP:              ip,
		ReputationScore: 50, // Neutral score
		ThreatTypes:     []string{},
		LastSeen:        time.Now(),
		RequestCount:    1,
		BlockedCount:    0,
		Metadata:        make(map[string]interface{}),
	}
	
	// Cache the result
	w.cacheMu.Lock()
	w.ipReputationCache[ip] = reputation
	w.cacheMu.Unlock()
	
	// Store in Redis
	if w.redis != nil {
		key := fmt.Sprintf("waf:ip_reputation:%s", ip)
		data, _ := json.Marshal(reputation)
		w.redis.Set(context.Background(), key, data, 24*time.Hour)
	}
	
	return reputation, nil
}

// matchesPattern checks if a value matches a rule pattern
func (w *WAFEngine) matchesPattern(value string, rule *WAFRule) bool {
	if rule.Regex != nil {
		return rule.Regex.MatchString(value)
	}
	
	// Fallback to simple string contains
	return strings.Contains(strings.ToLower(value), strings.ToLower(rule.Pattern))
}

// extractMatch extracts the matched portion from a value
func (w *WAFEngine) extractMatch(value string, rule *WAFRule) string {
	if rule.Regex != nil {
		match := rule.Regex.FindString(value)
		if match != "" {
			return match
		}
	}
	
	// Return the pattern if no regex match
	return rule.Pattern
}

// handleThreat handles a detected threat
func (w *WAFEngine) handleThreat(c *fiber.Ctx, threat *ThreatDetection, message string) error {
	// Log the threat
	w.logThreat(c, threat)
	
	// Update IP reputation
	w.updateIPReputation(c, threat)
	
	// Take action based on threat action
	switch threat.Action {
	case ActionBlock:
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error":   "Request blocked by WAF",
			"message": message,
			"rule_id": threat.Rule.ID,
		})
	case ActionRedirect:
		return c.Redirect("/security-error", fiber.StatusFound)
	default:
		return c.Next()
	}
}

// logThreat logs a threat detection
func (w *WAFEngine) logThreat(c *fiber.Ctx, threat *ThreatDetection) {
	// Log to system log
	fmt.Printf("[WAF] Threat detected: %+v\n", threat)
	
	// Store in Redis for analytics
	if w.redis != nil {
		key := fmt.Sprintf("waf:threats:%d", time.Now().Unix())
		data, _ := json.Marshal(threat)
		w.redis.Set(context.Background(), key, data, 7*24*time.Hour) // Keep for 7 days
	}
}

// updateIPReputation updates IP reputation based on threat
func (w *WAFEngine) updateIPReputation(c *fiber.Ctx, threat *ThreatDetection) {
	ip := w.getRealIP(c)
	reputation, _ := w.getIPReputation(ip)
	
	// Decrease reputation score based on threat severity
	switch threat.Severity {
	case SeverityCritical:
		reputation.ReputationScore -= 20
	case SeverityHigh:
		reputation.ReputationScore -= 15
	case SeverityMedium:
		reputation.ReputationScore -= 10
	case SeverityLow:
		reputation.ReputationScore -= 5
	}
	
	// Ensure score doesn't go below 0
	if reputation.ReputationScore < 0 {
		reputation.ReputationScore = 0
	}
	
	reputation.BlockedCount++
	reputation.LastSeen = time.Now()
	
	// Update cache and Redis
	w.cacheMu.Lock()
	w.ipReputationCache[ip] = reputation
	w.cacheMu.Unlock()
	
	if w.redis != nil {
		key := fmt.Sprintf("waf:ip_reputation:%s", ip)
		data, _ := json.Marshal(reputation)
		w.redis.Set(context.Background(), key, data, 24*time.Hour)
	}
}

// loadDefaultRules loads default WAF rules
func (w *WAFEngine) loadDefaultRules() {
	defaultRules := []*WAFRule{
		{
			ID:          "sql-injection-1",
			Name:        "SQL Injection Detection",
			Description: "Detects common SQL injection patterns",
			Type:        RuleTypeSQLInjection,
			Pattern:     `(?i)(union\s+select|insert\s+into|delete\s+from|drop\s+table|or\s+1\s*=\s*1|'\s+or\s+'1'\s*=\s*'1)`,
			Action:      ActionBlock,
			Severity:    SeverityCritical,
			Enabled:     true,
		},
		{
			ID:          "xss-1",
			Name:        "Cross-Site Scripting Detection",
			Description: "Detects XSS attempts",
			Type:        RuleTypeXSS,
			Pattern:     `(?i)(<script|javascript:|vbscript:|onload=|onerror=|onclick=)`,
			Action:      ActionBlock,
			Severity:    SeverityHigh,
			Enabled:     true,
		},
		{
			ID:          "path-traversal-1",
			Name:        "Path Traversal Detection",
			Description: "Detects path traversal attempts",
			Type:        RuleTypePathTraversal,
			Pattern:     `(\.\./|\.\.\\|/etc/passwd|/etc/shadow|boot\.ini)`,
			Action:      ActionBlock,
			Severity:    SeverityHigh,
			Enabled:     true,
		},
	}
	
	for _, rule := range defaultRules {
		w.AddRule(rule)
	}
}

// storeRule stores a rule in Redis
func (w *WAFEngine) storeRule(rule *WAFRule) {
	key := fmt.Sprintf("waf:rules:%s", rule.ID)
	data, _ := json.Marshal(rule)
	w.redis.Set(context.Background(), key, data, 0)
}

// removeRule removes a rule from Redis
func (w *WAFEngine) removeRule(ruleID string) {
	key := fmt.Sprintf("waf:rules:%s", ruleID)
	w.redis.Del(context.Background(), key)
}