package metrics

import (
	"sync"
	"sync/atomic"

	"github.com/cantte/go/assert"
	"github.com/cantte/go/prometheus/lazy"
	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	namespace string

	// CircuitBreakerRequests tracks the number of requests made to the circuit
	// breaker, labeled by service and the state the breaker was in when the
	// request arrived.
	//
	// Example usage:
	//   metrics.CircuitBreakerRequests.WithLabelValues("my_circuit_breaker", "open").Inc()
	CircuitBreakerRequests *lazy.CounterVec
	// CircuitBreakerErrorsTotal tracks the total number of requests the circuit
	// breaker rejected, labeled by service and rejection reason. Use this
	// counter to monitor how much traffic the breaker is shedding.
	//
	// Example usage:
	//   metrics.CircuitBreakerErrorsTotal.WithLabelValues("database", "tripped").Inc()
	CircuitBreakerErrorsTotal *lazy.CounterVec
}

// metricsByNamespace memoizes the metrics for a namespace. The lazy collectors
// register with prometheus on first use, so handing out two distinct Metrics
// for the same namespace would panic with a duplicate registration the moment
// both were used.
var metricsByNamespace sync.Map

var configuredMetrics atomic.Pointer[Metrics]

// Configure sets the metrics used by packages that emit shared internal
// metrics. Applications must call Configure with their application namespace
// before starting those packages.
//
// Calling Configure repeatedly with the same namespace is allowed. Attempting
// to replace an existing configuration with a different namespace returns an
// error because metrics may already be registered under the original name.
func Configure(namespace string) error {
	if err := assert.NotEmpty(namespace, "metrics: namespace is required"); err != nil {
		return err
	}

	configured := New(namespace)
	if configuredMetrics.CompareAndSwap(nil, configured) {
		return nil
	}

	return assert.Equal(
		configuredMetrics.Load().namespace,
		namespace,
		"metrics: already configured with a different namespace",
	)
}

// Configured returns the application-configured internal metrics. It returns
// nil when metrics have not been configured; recording methods are nil-safe.
func Configured() *Metrics {
	return configuredMetrics.Load()
}

func New(namespace string) *Metrics {
	if err := assert.NotEmpty(namespace, "metrics: namespace is required"); err != nil {
		panic(err)
	}

	if existing, ok := metricsByNamespace.Load(namespace); ok {
		return existing.(*Metrics)
	}

	created := &Metrics{
		namespace: namespace,
		CircuitBreakerRequests: lazy.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "circuitbreaker",
			Name:      "requests_total",
			Help:      "Tracks the number of requests made to the circuitbreaker by state.",
		}, []string{"service", "state"}),
		CircuitBreakerErrorsTotal: lazy.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "circuitbreaker",
			Name:      "errors_total",
			Help:      "Total number of requests rejected by the circuitbreaker, by service and reason.",
		}, []string{"service", "reason"}),
	}

	actual, _ := metricsByNamespace.LoadOrStore(namespace, created)
	return actual.(*Metrics)
}

// RecordRequest records a circuit breaker request. It is nil-safe so callers
// do not need conditional metric handling when instrumentation is disabled.
func (m *Metrics) RecordRequest(service, state string) {
	if m != nil {
		m.CircuitBreakerRequests.WithLabelValues(service, state).Inc()
	}
}

// RecordError records a request the circuit breaker rejected, where reason is
// why it was rejected (for example "tripped" or "too_many_requests"). It is
// nil-safe so callers do not need conditional metric handling when
// instrumentation is disabled.
func (m *Metrics) RecordError(service, reason string) {
	if m != nil {
		m.CircuitBreakerErrorsTotal.WithLabelValues(service, reason).Inc()
	}
}
