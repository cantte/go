package metrics

import (
	"sync"

	"github.com/cantte/go/prometheus/lazy"
	"github.com/prometheus/client_golang/prometheus"
)

// Standard histogram buckets for latency metrics in seconds.
var latencyBuckets = []float64{
	0.001, // 1ms
	0.002, // 2ms
	0.005, // 5ms
	0.01,  // 10ms
	0.02,  // 20ms
	0.05,  // 50ms
	0.1,   // 100ms
	0.2,   // 200ms
	0.3,   // 300ms
	0.4,   // 400ms
	0.5,   // 500ms
	0.75,  // 750ms
	1.0,   // 1s
	2.0,   // 2s
	3.0,   // 3s
	5.0,   // 5s
	10.0,  // 10s
}

// Metrics records database operation metrics for one Prometheus namespace.
// Instances are safe for concurrent use.
type Metrics struct {
	operationsLatency *lazy.HistogramVec
	operationsTotal   *lazy.CounterVec
}

var metricsByNamespace sync.Map

// New returns the shared database metric set for namespace. Passing an empty
// namespace creates metrics without a namespace prefix.
//
// Metric sets are shared by namespace to prevent duplicate collector
// registration when multiple database replicas use the same namespace.
func New(namespace string) *Metrics {
	if existing, ok := metricsByNamespace.Load(namespace); ok {
		return existing.(*Metrics)
	}

	created := &Metrics{
		operationsLatency: lazy.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: "database",
				Name:      "operations_latency_seconds",
				Help:      "Histogram of database operation latencies in seconds.",
				Buckets:   latencyBuckets,
			},
			[]string{"replica", "operation", "status"},
		),
		operationsTotal: lazy.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "database",
				Name:      "operations_total",
				Help:      "Total number of database operations processed.",
			},
			[]string{"replica", "operation", "status"},
		),
	}

	actual, _ := metricsByNamespace.LoadOrStore(namespace, created)
	return actual.(*Metrics)
}

// Observe records the duration and increments the count for a database
// operation.
func (m *Metrics) Observe(replica, operation, status string, durationSeconds float64) {
	if m == nil {
		return
	}

	m.operationsLatency.WithLabelValues(replica, operation, status).Observe(durationSeconds)
	m.operationsTotal.WithLabelValues(replica, operation, status).Inc()
}
