package security

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// IncidentSeverity defines the severity levels for security incidents
type IncidentSeverity string

const (
	SeverityCritical IncidentSeverity = "critical"
	SeverityHigh     IncidentSeverity = "high"
	SeverityMedium   IncidentSeverity = "medium"
	SeverityLow      IncidentSeverity = "low"
	SeverityInfo     IncidentSeverity = "info"
)

// IncidentType defines the types of security incidents
type IncidentType string

const (
	IncidentTypeDataBreach       IncidentType = "data_breach"
	IncidentTypeUnauthorizedAccess IncidentType = "unauthorized_access"
	IncidentTypeMalwareDetection  IncidentType = "malware_detection"
	IncidentTypeDDoSAttack       IncidentType = "ddos_attack"
	IncidentTypePhishingAttempt  IncidentType = "phishing_attempt"
	IncidentTypeInsiderThreat    IncidentType = "insider_threat"
	IncidentTypeSystemCompromise IncidentType = "system_compromise"
	IncidentTypeDataLeakage      IncidentType = "data_leakage"
	IncidentTypeRateLimitViolation IncidentType = "rate_limit_violation"
	IncidentTypeAuthenticationFailure IncidentType = "authentication_failure"
)

// IncidentStatus defines the status of a security incident
type IncidentStatus string

const (
	StatusNew        IncidentStatus = "new"
	StatusTriaged    IncidentStatus = "triaged"
	StatusInvestigating IncidentStatus = "investigating"
	StatusContained  IncidentStatus = "contained"
	StatusResolved   IncidentStatus = "resolved"
	StatusClosed     IncidentStatus = "closed"
)

// SecurityIncident represents a security incident
type SecurityIncident struct {
	ID          string           `json:"id"`
	Type        IncidentType     `json:"type"`
	Severity    IncidentSeverity `json:"severity"`
	Status      IncidentStatus   `json:"status"`
	Title       string           `json:"title"`
	Description string           `json:"description"`
	
	// Context information
	UserID     string            `json:"user_id,omitempty"`
	TenantID   string            `json:"tenant_id,omitempty"`
	IPAddress  string            `json:"ip_address,omitempty"`
	UserAgent  string            `json:"user_agent,omitempty"`
	Endpoint   string            `json:"endpoint,omitempty"`
	
	// Metadata
	Indicators  map[string]interface{} `json:"indicators"`
	Evidence    []Evidence            `json:"evidence"`
	Actions     []IncidentAction      `json:"actions"`
	
	// Timestamps
	DetectedAt  time.Time `json:"detected_at"`
	TriagedAt   *time.Time `json:"triaged_at,omitempty"`
	ResolvedAt  *time.Time `json:"resolved_at,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	
	// Assignment
	AssignedTo  string `json:"assigned_to,omitempty"`
	
	// Impact assessment
	ImpactAssessment *ImpactAssessment `json:"impact_assessment,omitempty"`
}

// Evidence represents evidence related to a security incident
type Evidence struct {
	Type        string                 `json:"type"`
	Source      string                 `json:"source"`
	Data        map[string]interface{} `json:"data"`
	CollectedAt time.Time              `json:"collected_at"`
	Hash        string                 `json:"hash"` // For integrity verification
}

// IncidentAction represents an action taken in response to an incident
type IncidentAction struct {
	Type        string                 `json:"type"`
	Description string                 `json:"description"`
	PerformedBy string                 `json:"performed_by"`
	PerformedAt time.Time              `json:"performed_at"`
	Result      string                 `json:"result"`
	Details     map[string]interface{} `json:"details"`
}

// ImpactAssessment represents the impact assessment of an incident
type ImpactAssessment struct {
	AffectedUsers     int      `json:"affected_users"`
	AffectedTenants   int      `json:"affected_tenants"`
	DataCompromised   bool     `json:"data_compromised"`
	ServiceImpact     string   `json:"service_impact"`
	FinancialImpact   *float64 `json:"financial_impact,omitempty"`
	ComplianceImpact  []string `json:"compliance_impact"`
	ReputationImpact  string   `json:"reputation_impact"`
}

// IncidentResponseService manages security incident response
type IncidentResponseService struct {
	logger        *zap.Logger
	storage       IncidentStorage
	notifier      IncidentNotifier
	playbooks     map[IncidentType]*ResponsePlaybook
	escalationRules *EscalationRules
	metrics       *IncidentMetrics
}

// IncidentStorage defines the interface for incident storage
type IncidentStorage interface {
	Store(ctx context.Context, incident *SecurityIncident) error
	Get(ctx context.Context, id string) (*SecurityIncident, error)
	List(ctx context.Context, filters IncidentFilters) ([]*SecurityIncident, error)
	Update(ctx context.Context, incident *SecurityIncident) error
}

// IncidentNotifier defines the interface for incident notifications
type IncidentNotifier interface {
	NotifyIncident(ctx context.Context, incident *SecurityIncident) error
	NotifyEscalation(ctx context.Context, incident *SecurityIncident, escalationLevel int) error
	NotifyResolution(ctx context.Context, incident *SecurityIncident) error
}

// IncidentFilters defines filters for incident queries
type IncidentFilters struct {
	Severity   []IncidentSeverity `json:"severity"`
	Type       []IncidentType     `json:"type"`
	Status     []IncidentStatus   `json:"status"`
	TenantID   string             `json:"tenant_id"`
	AssignedTo string             `json:"assigned_to"`
	DateFrom   *time.Time         `json:"date_from"`
	DateTo     *time.Time         `json:"date_to"`
}

// ResponsePlaybook defines automated response actions for incident types
type ResponsePlaybook struct {
	IncidentType     IncidentType        `json:"incident_type"`
	AutoActions      []AutomatedAction   `json:"auto_actions"`
	ManualActions    []ManualAction      `json:"manual_actions"`
	EscalationTime   time.Duration       `json:"escalation_time"`
	RequiredEvidence []string           `json:"required_evidence"`
}

// AutomatedAction defines an automated response action
type AutomatedAction struct {
	Type        string                 `json:"type"`
	Description string                 `json:"description"`
	Conditions  map[string]interface{} `json:"conditions"`
	Parameters  map[string]interface{} `json:"parameters"`
	Timeout     time.Duration          `json:"timeout"`
}

// ManualAction defines a manual response action
type ManualAction struct {
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Instructions string  `json:"instructions"`
	RequiredRole string  `json:"required_role"`
	Priority    int      `json:"priority"`
}

// EscalationRules defines when and how to escalate incidents
type EscalationRules struct {
	TimeBasedEscalation map[IncidentSeverity]time.Duration `json:"time_based_escalation"`
	SeverityEscalation  map[IncidentType]IncidentSeverity  `json:"severity_escalation"`
	NotificationChains  map[int][]string                   `json:"notification_chains"`
}

// IncidentMetrics tracks incident response metrics
type IncidentMetrics struct {
	TotalIncidents       int64         `json:"total_incidents"`
	IncidentsByType      map[IncidentType]int64 `json:"incidents_by_type"`
	IncidentsBySeverity  map[IncidentSeverity]int64 `json:"incidents_by_severity"`
	AverageResponseTime  time.Duration `json:"average_response_time"`
	AverageResolutionTime time.Duration `json:"average_resolution_time"`
	EscalationRate       float64       `json:"escalation_rate"`
}

// NewIncidentResponseService creates a new incident response service
func NewIncidentResponseService(
	logger *zap.Logger,
	storage IncidentStorage,
	notifier IncidentNotifier,
) *IncidentResponseService {
	service := &IncidentResponseService{
		logger:   logger,
		storage:  storage,
		notifier: notifier,
		playbooks: make(map[IncidentType]*ResponsePlaybook),
		escalationRules: &EscalationRules{
			TimeBasedEscalation: map[IncidentSeverity]time.Duration{
				SeverityCritical: 15 * time.Minute,
				SeverityHigh:     1 * time.Hour,
				SeverityMedium:   4 * time.Hour,
				SeverityLow:      24 * time.Hour,
			},
			NotificationChains: map[int][]string{
				1: {"security-team@company.com"},
				2: {"security-manager@company.com", "ciso@company.com"},
				3: {"ceo@company.com", "legal@company.com"},
			},
		},
		metrics: &IncidentMetrics{
			IncidentsByType:     make(map[IncidentType]int64),
			IncidentsBySeverity: make(map[IncidentSeverity]int64),
		},
	}

	// Initialize default playbooks
	service.initializeDefaultPlaybooks()
	
	return service
}

// CreateIncident creates a new security incident
func (s *IncidentResponseService) CreateIncident(ctx context.Context, req *CreateIncidentRequest) (*SecurityIncident, error) {
	incident := &SecurityIncident{
		ID:          generateIncidentID(),
		Type:        req.Type,
		Severity:    req.Severity,
		Status:      StatusNew,
		Title:       req.Title,
		Description: req.Description,
		UserID:      req.UserID,
		TenantID:    req.TenantID,
		IPAddress:   req.IPAddress,
		UserAgent:   req.UserAgent,
		Endpoint:    req.Endpoint,
		Indicators:  req.Indicators,
		Evidence:    req.Evidence,
		DetectedAt:  time.Now(),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// Store incident
	if err := s.storage.Store(ctx, incident); err != nil {
		return nil, fmt.Errorf("failed to store incident: %w", err)
	}

	// Execute automated response
	if err := s.executeAutomatedResponse(ctx, incident); err != nil {
		s.logger.Error("Failed to execute automated response",
			zap.String("incident_id", incident.ID),
			zap.Error(err),
		)
	}

	// Send notifications
	if err := s.notifier.NotifyIncident(ctx, incident); err != nil {
		s.logger.Error("Failed to send incident notification",
			zap.String("incident_id", incident.ID),
			zap.Error(err),
		)
	}

	// Update metrics
	s.updateMetrics(incident)

	s.logger.Info("Security incident created",
		zap.String("incident_id", incident.ID),
		zap.String("type", string(incident.Type)),
		zap.String("severity", string(incident.Severity)),
	)

	return incident, nil
}

// CreateIncidentRequest represents a request to create an incident
type CreateIncidentRequest struct {
	Type        IncidentType           `json:"type"`
	Severity    IncidentSeverity       `json:"severity"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	UserID      string                 `json:"user_id,omitempty"`
	TenantID    string                 `json:"tenant_id,omitempty"`
	IPAddress   string                 `json:"ip_address,omitempty"`
	UserAgent   string                 `json:"user_agent,omitempty"`
	Endpoint    string                 `json:"endpoint,omitempty"`
	Indicators  map[string]interface{} `json:"indicators"`
	Evidence    []Evidence             `json:"evidence"`
}

// executeAutomatedResponse executes automated response actions
func (s *IncidentResponseService) executeAutomatedResponse(ctx context.Context, incident *SecurityIncident) error {
	playbook, exists := s.playbooks[incident.Type]
	if !exists {
		return nil // No playbook defined
	}

	for _, action := range playbook.AutoActions {
		if s.shouldExecuteAction(incident, action) {
			if err := s.executeAction(ctx, incident, action); err != nil {
				s.logger.Error("Failed to execute automated action",
					zap.String("incident_id", incident.ID),
					zap.String("action_type", action.Type),
					zap.Error(err),
				)
				continue
			}

			// Record action
			incidentAction := IncidentAction{
				Type:        action.Type,
				Description: action.Description,
				PerformedBy: "system",
				PerformedAt: time.Now(),
				Result:      "success",
				Details:     action.Parameters,
			}
			incident.Actions = append(incident.Actions, incidentAction)
		}
	}

	return s.storage.Update(ctx, incident)
}

// shouldExecuteAction determines if an automated action should be executed
func (s *IncidentResponseService) shouldExecuteAction(incident *SecurityIncident, action AutomatedAction) bool {
	// Check conditions
	for key, expectedValue := range action.Conditions {
		switch key {
		case "severity":
			if incident.Severity != IncidentSeverity(expectedValue.(string)) {
				return false
			}
		case "user_id_present":
			if expectedValue.(bool) && incident.UserID == "" {
				return false
			}
		case "ip_address_present":
			if expectedValue.(bool) && incident.IPAddress == "" {
				return false
			}
		}
	}
	return true
}

// executeAction executes a specific automated action
func (s *IncidentResponseService) executeAction(ctx context.Context, incident *SecurityIncident, action AutomatedAction) error {
	switch action.Type {
	case "block_ip":
		return s.blockIPAddress(ctx, incident.IPAddress)
	case "disable_user":
		return s.disableUser(ctx, incident.UserID)
	case "revoke_sessions":
		return s.revokeSessions(ctx, incident.UserID)
	case "quarantine_tenant":
		return s.quarantineTenant(ctx, incident.TenantID)
	case "enable_enhanced_monitoring":
		return s.enableEnhancedMonitoring(ctx, incident)
	default:
		return fmt.Errorf("unknown action type: %s", action.Type)
	}
}

// Automated response actions
func (s *IncidentResponseService) blockIPAddress(ctx context.Context, ipAddress string) error {
	s.logger.Info("Blocking IP address", zap.String("ip", ipAddress))
	// Implementation: Add IP to firewall block list
	return nil
}

func (s *IncidentResponseService) disableUser(ctx context.Context, userID string) error {
	s.logger.Info("Disabling user account", zap.String("user_id", userID))
	// Implementation: Disable user account in database
	return nil
}

func (s *IncidentResponseService) revokeSessions(ctx context.Context, userID string) error {
	s.logger.Info("Revoking user sessions", zap.String("user_id", userID))
	// Implementation: Revoke all active sessions for user
	return nil
}

func (s *IncidentResponseService) quarantineTenant(ctx context.Context, tenantID string) error {
	s.logger.Info("Quarantining tenant", zap.String("tenant_id", tenantID))
	// Implementation: Restrict tenant access
	return nil
}

func (s *IncidentResponseService) enableEnhancedMonitoring(ctx context.Context, incident *SecurityIncident) error {
	s.logger.Info("Enabling enhanced monitoring", zap.String("incident_id", incident.ID))
	// Implementation: Increase monitoring sensitivity
	return nil
}

// UpdateIncident updates an existing incident
func (s *IncidentResponseService) UpdateIncident(ctx context.Context, id string, updates *IncidentUpdate) (*SecurityIncident, error) {
	incident, err := s.storage.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get incident: %w", err)
	}

	// Apply updates
	if updates.Status != nil {
		incident.Status = *updates.Status
		if *updates.Status == StatusTriaged && incident.TriagedAt == nil {
			now := time.Now()
			incident.TriagedAt = &now
		}
		if *updates.Status == StatusResolved && incident.ResolvedAt == nil {
			now := time.Now()
			incident.ResolvedAt = &now
		}
	}

	if updates.AssignedTo != nil {
		incident.AssignedTo = *updates.AssignedTo
	}

	if updates.ImpactAssessment != nil {
		incident.ImpactAssessment = updates.ImpactAssessment
	}

	if len(updates.Actions) > 0 {
		incident.Actions = append(incident.Actions, updates.Actions...)
	}

	incident.UpdatedAt = time.Now()

	// Store updated incident
	if err := s.storage.Update(ctx, incident); err != nil {
		return nil, fmt.Errorf("failed to update incident: %w", err)
	}

	return incident, nil
}

// IncidentUpdate represents updates to an incident
type IncidentUpdate struct {
	Status           *IncidentStatus    `json:"status,omitempty"`
	AssignedTo       *string           `json:"assigned_to,omitempty"`
	ImpactAssessment *ImpactAssessment `json:"impact_assessment,omitempty"`
	Actions          []IncidentAction  `json:"actions,omitempty"`
}

// initializeDefaultPlaybooks sets up default response playbooks
func (s *IncidentResponseService) initializeDefaultPlaybooks() {
	// Data breach playbook
	s.playbooks[IncidentTypeDataBreach] = &ResponsePlaybook{
		IncidentType: IncidentTypeDataBreach,
		AutoActions: []AutomatedAction{
			{
				Type:        "enable_enhanced_monitoring",
				Description: "Enable enhanced monitoring for affected systems",
				Conditions:  map[string]interface{}{"severity": "critical"},
			},
		},
		ManualActions: []ManualAction{
			{
				Type:         "legal_notification",
				Description:  "Notify legal team for compliance assessment",
				Instructions: "Contact legal team immediately for GDPR/CCPA assessment",
				RequiredRole: "security_manager",
				Priority:     1,
			},
		},
		EscalationTime: 15 * time.Minute,
	}

	// Unauthorized access playbook
	s.playbooks[IncidentTypeUnauthorizedAccess] = &ResponsePlaybook{
		IncidentType: IncidentTypeUnauthorizedAccess,
		AutoActions: []AutomatedAction{
			{
				Type:        "revoke_sessions",
				Description: "Revoke all active sessions for affected user",
				Conditions:  map[string]interface{}{"user_id_present": true},
			},
			{
				Type:        "block_ip",
				Description: "Block source IP address",
				Conditions:  map[string]interface{}{"ip_address_present": true},
			},
		},
		EscalationTime: 30 * time.Minute,
	}

	// DDoS attack playbook
	s.playbooks[IncidentTypeDDoSAttack] = &ResponsePlaybook{
		IncidentType: IncidentTypeDDoSAttack,
		AutoActions: []AutomatedAction{
			{
				Type:        "enable_rate_limiting",
				Description: "Enable aggressive rate limiting",
				Conditions:  map[string]interface{}{"severity": "high"},
			},
		},
		EscalationTime: 5 * time.Minute,
	}
}

// updateMetrics updates incident response metrics
func (s *IncidentResponseService) updateMetrics(incident *SecurityIncident) {
	s.metrics.TotalIncidents++
	s.metrics.IncidentsByType[incident.Type]++
	s.metrics.IncidentsBySeverity[incident.Severity]++
}

// GetMetrics returns current incident response metrics
func (s *IncidentResponseService) GetMetrics() *IncidentMetrics {
	return s.metrics
}

// Utility function to generate incident IDs
func generateIncidentID() string {
	return fmt.Sprintf("INC-%d-%d", time.Now().Unix(), time.Now().Nanosecond()%1000)
}