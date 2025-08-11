package audit

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
)

// SIEMConfig holds configuration for SIEM integration
type SIEMConfig struct {
	Type          string `env:"SIEM_TYPE" envDefault:""`           // elasticsearch, splunk, sumo, custom
	URL           string `env:"SIEM_URL"`                          // SIEM endpoint URL
	APIKey        string `env:"SIEM_API_KEY"`                      // API key for authentication
	Username      string `env:"SIEM_USERNAME"`                     // Username for basic auth
	Password      string `env:"SIEM_PASSWORD"`                     // Password for basic auth
	Index         string `env:"SIEM_INDEX" envDefault:"pyairtable-audit"` // Index/collection name
	BatchSize     int    `env:"SIEM_BATCH_SIZE" envDefault:"100"`  // Batch size for bulk operations
	FlushInterval int    `env:"SIEM_FLUSH_INTERVAL" envDefault:"30"` // Flush interval in seconds
	TLSEnabled    bool   `env:"SIEM_TLS_ENABLED" envDefault:"true"` // Enable TLS
	TLSSkipVerify bool   `env:"SIEM_TLS_SKIP_VERIFY" envDefault:"false"` // Skip TLS verification
}

// SIEMIntegrator handles integration with various SIEM systems
type SIEMIntegrator struct {
	config        *SIEMConfig
	httpClient    *http.Client
	esClient      *elasticsearch.Client
	eventBuffer   []*AuditEvent
	stopCh        chan struct{}
}

// NewSIEMIntegrator creates a new SIEM integrator
func NewSIEMIntegrator(config *SIEMConfig) (*SIEMIntegrator, error) {
	integrator := &SIEMIntegrator{
		config:      config,
		eventBuffer: make([]*AuditEvent, 0),
		stopCh:      make(chan struct{}),
	}

	// Configure HTTP client with TLS settings
	tlsConfig := &tls.Config{
		InsecureSkipVerify: config.TLSSkipVerify,
	}

	integrator.httpClient = &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}

	// Initialize specific SIEM client if needed
	if err := integrator.initializeSIEMClient(); err != nil {
		return nil, fmt.Errorf("failed to initialize SIEM client: %w", err)
	}

	// Start background processor
	go integrator.backgroundProcessor()

	return integrator, nil
}

// initializeSIEMClient initializes the specific SIEM client
func (s *SIEMIntegrator) initializeSIEMClient() error {
	switch strings.ToLower(s.config.Type) {
	case "elasticsearch":
		return s.initElasticsearch()
	case "splunk":
		// Splunk uses HTTP Event Collector, no special client needed
		return nil
	case "sumo":
		// Sumo Logic uses HTTP endpoint, no special client needed
		return nil
	case "custom":
		// Custom endpoint, no special client needed
		return nil
	default:
		if s.config.Type != "" {
			return fmt.Errorf("unsupported SIEM type: %s", s.config.Type)
		}
	}
	return nil
}

// initElasticsearch initializes Elasticsearch client
func (s *SIEMIntegrator) initElasticsearch() error {
	cfg := elasticsearch.Config{
		Addresses: []string{s.config.URL},
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: s.config.TLSSkipVerify,
			},
		},
	}

	// Configure authentication
	if s.config.APIKey != "" {
		cfg.APIKey = s.config.APIKey
	} else if s.config.Username != "" && s.config.Password != "" {
		cfg.Username = s.config.Username
		cfg.Password = s.config.Password
	}

	client, err := elasticsearch.NewClient(cfg)
	if err != nil {
		return fmt.Errorf("failed to create Elasticsearch client: %w", err)
	}

	s.esClient = client

	// Test connection
	res, err := client.Info()
	if err != nil {
		return fmt.Errorf("failed to connect to Elasticsearch: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("Elasticsearch connection error: %s", res.Status())
	}

	return nil
}

// SendEvent sends a single audit event to SIEM
func (s *SIEMIntegrator) SendEvent(ctx context.Context, event *AuditEvent) error {
	if s.config.Type == "" {
		return nil // SIEM not configured
	}

	siemEvent := s.convertToSIEMFormat(event)

	switch strings.ToLower(s.config.Type) {
	case "elasticsearch":
		return s.sendToElasticsearch(ctx, siemEvent)
	case "splunk":
		return s.sendToSplunk(ctx, siemEvent)
	case "sumo":
		return s.sendToSumoLogic(ctx, siemEvent)
	case "custom":
		return s.sendToCustomEndpoint(ctx, siemEvent)
	default:
		return fmt.Errorf("unsupported SIEM type: %s", s.config.Type)
	}
}

// SendBatch sends multiple audit events to SIEM in batch
func (s *SIEMIntegrator) SendBatch(ctx context.Context, events []*AuditEvent) error {
	if s.config.Type == "" || len(events) == 0 {
		return nil
	}

	siemEvents := make([]map[string]interface{}, len(events))
	for i, event := range events {
		siemEvents[i] = s.convertToSIEMFormat(event)
	}

	switch strings.ToLower(s.config.Type) {
	case "elasticsearch":
		return s.sendBatchToElasticsearch(ctx, siemEvents)
	case "splunk":
		return s.sendBatchToSplunk(ctx, siemEvents)
	case "sumo":
		return s.sendBatchToSumoLogic(ctx, siemEvents)
	case "custom":
		return s.sendBatchToCustomEndpoint(ctx, siemEvents)
	default:
		return fmt.Errorf("unsupported SIEM type: %s", s.config.Type)
	}
}

// convertToSIEMFormat converts AuditEvent to SIEM-compatible format
func (s *SIEMIntegrator) convertToSIEMFormat(event *AuditEvent) map[string]interface{} {
	siemEvent := map[string]interface{}{
		"@timestamp":     event.Timestamp.UTC().Format(time.RFC3339),
		"source":         "pyairtable-compose",
		"environment":    os.Getenv("ENVIRONMENT"),
		"service_name":   event.ServiceName,
		"event_type":     event.EventType,
		"severity":       event.Severity,
		"message":        event.Message,
		"result":         event.Result,
		"user_id":        event.UserID,
		"tenant_id":      event.TenantID,
		"session_id":     event.SessionID,
		"ip_address":     event.IPAddress,
		"user_agent":     event.UserAgent,
		"resource":       event.Resource,
		"action":         event.Action,
		"request_id":     event.RequestID,
		"event_id":       event.ID,
		"integrity_hash": event.Signature,
	}

	// Add details if present
	if len(event.Details) > 0 {
		var details map[string]interface{}
		if err := json.Unmarshal(event.Details, &details); err == nil {
			siemEvent["details"] = details
		}
	}

	// Add SIEM-specific metadata
	siemEvent["host"] = getHostname()
	siemEvent["tags"] = []string{"audit", "security", "pyairtable"}

	return siemEvent
}

// sendToElasticsearch sends event to Elasticsearch
func (s *SIEMIntegrator) sendToElasticsearch(ctx context.Context, event map[string]interface{}) error {
	if s.esClient == nil {
		return fmt.Errorf("Elasticsearch client not initialized")
	}

	eventJSON, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Index the document
	req := esapi.IndexRequest{
		Index: s.config.Index,
		Body:  bytes.NewReader(eventJSON),
	}

	res, err := req.Do(ctx, s.esClient)
	if err != nil {
		return fmt.Errorf("failed to index document: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("Elasticsearch error: %s", res.Status())
	}

	return nil
}

// sendBatchToElasticsearch sends multiple events to Elasticsearch using bulk API
func (s *SIEMIntegrator) sendBatchToElasticsearch(ctx context.Context, events []map[string]interface{}) error {
	if s.esClient == nil {
		return fmt.Errorf("Elasticsearch client not initialized")
	}

	var bulkBody bytes.Buffer
	for _, event := range events {
		// Add index action
		indexAction := map[string]interface{}{
			"index": map[string]interface{}{
				"_index": s.config.Index,
			},
		}
		actionJSON, _ := json.Marshal(indexAction)
		bulkBody.Write(actionJSON)
		bulkBody.WriteByte('\n')

		// Add document
		eventJSON, _ := json.Marshal(event)
		bulkBody.Write(eventJSON)
		bulkBody.WriteByte('\n')
	}

	req := esapi.BulkRequest{
		Body: &bulkBody,
	}

	res, err := req.Do(ctx, s.esClient)
	if err != nil {
		return fmt.Errorf("failed to bulk index: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("Elasticsearch bulk error: %s", res.Status())
	}

	return nil
}

// sendToSplunk sends event to Splunk HTTP Event Collector
func (s *SIEMIntegrator) sendToSplunk(ctx context.Context, event map[string]interface{}) error {
	splunkEvent := map[string]interface{}{
		"time":   event["@timestamp"],
		"source": "pyairtable-compose",
		"event":  event,
	}

	return s.sendHTTPEvent(ctx, s.config.URL, splunkEvent, map[string]string{
		"Authorization": "Splunk " + s.config.APIKey,
		"Content-Type":  "application/json",
	})
}

// sendBatchToSplunk sends multiple events to Splunk
func (s *SIEMIntegrator) sendBatchToSplunk(ctx context.Context, events []map[string]interface{}) error {
	for _, event := range events {
		if err := s.sendToSplunk(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

// sendToSumoLogic sends event to Sumo Logic
func (s *SIEMIntegrator) sendToSumoLogic(ctx context.Context, event map[string]interface{}) error {
	return s.sendHTTPEvent(ctx, s.config.URL, event, map[string]string{
		"Content-Type": "application/json",
	})
}

// sendBatchToSumoLogic sends multiple events to Sumo Logic
func (s *SIEMIntegrator) sendBatchToSumoLogic(ctx context.Context, events []map[string]interface{}) error {
	return s.sendHTTPEvent(ctx, s.config.URL, events, map[string]string{
		"Content-Type": "application/json",
	})
}

// sendToCustomEndpoint sends event to custom HTTP endpoint
func (s *SIEMIntegrator) sendToCustomEndpoint(ctx context.Context, event map[string]interface{}) error {
	headers := map[string]string{
		"Content-Type": "application/json",
	}

	if s.config.APIKey != "" {
		headers["Authorization"] = "Bearer " + s.config.APIKey
	}

	return s.sendHTTPEvent(ctx, s.config.URL, event, headers)
}

// sendBatchToCustomEndpoint sends multiple events to custom endpoint
func (s *SIEMIntegrator) sendBatchToCustomEndpoint(ctx context.Context, events []map[string]interface{}) error {
	headers := map[string]string{
		"Content-Type": "application/json",
	}

	if s.config.APIKey != "" {
		headers["Authorization"] = "Bearer " + s.config.APIKey
	}

	return s.sendHTTPEvent(ctx, s.config.URL, events, headers)
}

// sendHTTPEvent sends event via HTTP POST
func (s *SIEMIntegrator) sendHTTPEvent(ctx context.Context, url string, payload interface{}, headers map[string]string) error {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(payloadJSON))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("SIEM endpoint returned error: %d", resp.StatusCode)
	}

	return nil
}

// backgroundProcessor processes buffered events periodically
func (s *SIEMIntegrator) backgroundProcessor() {
	ticker := time.NewTicker(time.Duration(s.config.FlushInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if len(s.eventBuffer) > 0 {
				events := make([]*AuditEvent, len(s.eventBuffer))
				copy(events, s.eventBuffer)
				s.eventBuffer = s.eventBuffer[:0]

				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				if err := s.SendBatch(ctx, events); err != nil {
					fmt.Printf("Failed to send events to SIEM: %v\n", err)
					// Re-add events to buffer for retry
					s.eventBuffer = append(s.eventBuffer, events...)
				}
				cancel()
			}
		case <-s.stopCh:
			return
		}
	}
}

// BufferEvent adds an event to the buffer for batch processing
func (s *SIEMIntegrator) BufferEvent(event *AuditEvent) {
	s.eventBuffer = append(s.eventBuffer, event)
	
	// Force flush if buffer is full
	if len(s.eventBuffer) >= s.config.BatchSize {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			
			events := make([]*AuditEvent, len(s.eventBuffer))
			copy(events, s.eventBuffer)
			s.eventBuffer = s.eventBuffer[:0]
			
			if err := s.SendBatch(ctx, events); err != nil {
				fmt.Printf("Failed to send buffered events to SIEM: %v\n", err)
			}
		}()
	}
}

// Close stops the SIEM integrator and flushes remaining events
func (s *SIEMIntegrator) Close() error {
	close(s.stopCh)
	
	// Flush remaining events
	if len(s.eventBuffer) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		
		if err := s.SendBatch(ctx, s.eventBuffer); err != nil {
			return fmt.Errorf("failed to flush remaining events: %w", err)
		}
	}
	
	return nil
}

// getHostname returns the hostname for event metadata
func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}