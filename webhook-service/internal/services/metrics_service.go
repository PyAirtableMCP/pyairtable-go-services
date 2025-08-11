package services

import (
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// MetricsService handles Prometheus metrics for webhooks
type MetricsService struct {
	// Counter metrics
	webhookDeliveryAttempts *prometheus.CounterVec
	webhookDeliverySuccesses *prometheus.CounterVec
	webhookDeliveryFailures *prometheus.CounterVec
	
	// Histogram metrics
	webhookDeliveryDuration *prometheus.HistogramVec
	
	// Gauge metrics
	webhookActiveCount      prometheus.Gauge
	webhookDeadLetterCount  *prometheus.GaugeVec
	circuitBreakerState     *prometheus.GaugeVec
}

// NewMetricsService creates a new metrics service
func NewMetricsService() *MetricsService {
	return &MetricsService{
		webhookDeliveryAttempts: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "webhook_delivery_attempts_total",
				Help: "Total number of webhook delivery attempts",
			},
			[]string{"webhook_id", "event_type", "status"},
		),
		
		webhookDeliverySuccesses: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "webhook_delivery_successes_total",
				Help: "Total number of successful webhook deliveries",
			},
			[]string{"webhook_id", "event_type"},
		),
		
		webhookDeliveryFailures: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "webhook_delivery_failures_total",
				Help: "Total number of failed webhook deliveries",
			},
			[]string{"webhook_id", "event_type", "failure_reason"},
		),
		
		webhookDeliveryDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name: "webhook_delivery_duration_seconds",
				Help: "Time spent delivering webhooks",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"webhook_id", "event_type", "status"},
		),
		
		webhookActiveCount: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "webhook_active_total",
				Help: "Total number of active webhooks",
			},
		),
		
		webhookDeadLetterCount: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "webhook_dead_letters_total",
				Help: "Total number of dead letters per webhook",
			},
			[]string{"webhook_id"},
		),
		
		circuitBreakerState: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "webhook_circuit_breaker_state",
				Help: "Circuit breaker state (0=closed, 1=half-open, 2=open)",
			},
			[]string{"webhook_id"},
		),
	}
}

// RecordDeliveryAttempt records a webhook delivery attempt
func (m *MetricsService) RecordDeliveryAttempt(webhookID uint, eventType, status string) {
	m.webhookDeliveryAttempts.WithLabelValues(
		strconv.FormatUint(uint64(webhookID), 10),
		eventType,
		status,
	).Inc()
}

// RecordDeliverySuccess records a successful webhook delivery
func (m *MetricsService) RecordDeliverySuccess(webhookID uint, eventType string, duration time.Duration) {
	webhookIDStr := strconv.FormatUint(uint64(webhookID), 10)
	
	m.webhookDeliverySuccesses.WithLabelValues(webhookIDStr, eventType).Inc()
	m.webhookDeliveryDuration.WithLabelValues(webhookIDStr, eventType, "success").Observe(duration.Seconds())
}

// RecordDeliveryFailure records a failed webhook delivery
func (m *MetricsService) RecordDeliveryFailure(webhookID uint, eventType, failureReason string, duration time.Duration) {
	webhookIDStr := strconv.FormatUint(uint64(webhookID), 10)
	
	m.webhookDeliveryFailures.WithLabelValues(webhookIDStr, eventType, failureReason).Inc()
	m.webhookDeliveryDuration.WithLabelValues(webhookIDStr, eventType, "failure").Observe(duration.Seconds())
}

// SetActiveWebhookCount sets the total number of active webhooks
func (m *MetricsService) SetActiveWebhookCount(count int) {
	m.webhookActiveCount.Set(float64(count))
}

// SetDeadLetterCount sets the dead letter count for a webhook
func (m *MetricsService) SetDeadLetterCount(webhookID uint, count int) {
	m.webhookDeadLetterCount.WithLabelValues(
		strconv.FormatUint(uint64(webhookID), 10),
	).Set(float64(count))
}

// SetCircuitBreakerState sets the circuit breaker state for a webhook
func (m *MetricsService) SetCircuitBreakerState(webhookID uint, state string) {
	var stateValue float64
	switch state {
	case "closed":
		stateValue = 0
	case "half-open":
		stateValue = 1
	case "open":
		stateValue = 2
	}
	
	m.circuitBreakerState.WithLabelValues(
		strconv.FormatUint(uint64(webhookID), 10),
	).Set(stateValue)
}

// GetMetricsHandler returns the Prometheus metrics handler
func (m *MetricsService) GetMetricsHandler() prometheus.Handler {
	return prometheus.Handler()
}