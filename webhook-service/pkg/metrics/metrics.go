package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Registry struct {
	HTTPRequestsTotal    *prometheus.CounterVec
	HTTPRequestDuration  *prometheus.HistogramVec
	DatabaseConnections  prometheus.Gauge
	RedisConnections     prometheus.Gauge
}

func NewRegistry() *Registry {
	return &Registry{
		HTTPRequestsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "webhookservice_http_requests_total",
				Help: "Total number of HTTP requests",
			},
			[]string{"method", "endpoint", "status"},
		),
		HTTPRequestDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "webhookservice_http_request_duration_seconds",
				Help:    "Duration of HTTP requests in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"method", "endpoint"},
		),
		DatabaseConnections: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "webhookservice_database_connections",
				Help: "Number of active database connections",
			},
		),
		RedisConnections: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "webhookservice_redis_connections",
				Help: "Number of active Redis connections",
			},
		),
	}
}