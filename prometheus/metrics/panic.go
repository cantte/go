package metrics

import (
	"sync"
	"sync/atomic"

	"github.com/cantte/go/assert"
	"github.com/cantte/go/prometheus/lazy"
	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	// PanicsTotal tracks panics recovered by library components.
	//
	// Labels:
	//   - "caller": The function or component that recovered the panic
	//   - "context": Additional context about where the panic occurred
	//
	// Example usage:
	//   metrics.RecordPanic("repeat.Every", "background")
	PanicsTotal *lazy.CounterVec
}

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
		configuredMetrics.Load(),
		configured,
		"metrics: already configured with a different namespace",
	)
}

// Configured returns the application-configured internal metrics. It panics
// with a setup error when Configure has not been called, preventing ambiguous
// unnamespaced metrics from being emitted.
func Configured() *Metrics {
	configured := configuredMetrics.Load()
	if err := assert.NotNil(
		configured,
		"metrics: Configure must be called with an application namespace",
	); err != nil {
		panic(err)
	}
	return configured
}

func New(namespace string) *Metrics {
	if err := assert.NotEmpty(namespace, "metrics: namespace is required"); err != nil {
		panic(err)
	}

	if existing, ok := metricsByNamespace.Load(namespace); ok {
		return existing.(*Metrics)
	}

	created := &Metrics{
		PanicsTotal: lazy.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "internal",
			Name:      "panics_total",
			Help:      "Total number of panics recovered by library components.",
		}, []string{"caller", "context"}),
	}

	actual, _ := metricsByNamespace.LoadOrStore(namespace, created)
	return actual.(*Metrics)
}

// RecordPanic records a recovered panic. It is nil-safe so callers do not need
// conditional metric handling when instrumentation is disabled.
func (m *Metrics) RecordPanic(caller, context string) {
	if m != nil {
		m.PanicsTotal.WithLabelValues(caller, context).Inc()
	}
}
