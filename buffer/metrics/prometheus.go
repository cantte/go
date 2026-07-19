package metrics

import (
	"sync"

	"github.com/cantte/go/prometheus/lazy"
	"github.com/prometheus/client_golang/prometheus"
)

// BufferMetrics contains the pre-bound metric handles for one buffer. Keeping
// these handles avoids a label lookup on every buffered item.
type BufferMetrics struct {
	buffered prometheus.Counter
	dropped  prometheus.Counter
	closed   prometheus.Counter
	size     prometheus.Gauge
}

type Metrics struct {
	// BufferState is a counter to track the number of times a buffer is used and
	// what state is triggered.
	//
	// Possible states are:
	// - "buffered": The item was added to the buffer.
	// - "dropped": The item was dropped because the buffer was full.
	// - "closed": An insert was rejected because the buffer was closing or closed.
	//
	// Example usage:
	//   metrics.BufferInserts.WithLabelValues(b.String(), "buffered").Inc()
	BufferState *lazy.CounterVec
	// BufferSize is a gauge to track the fill percentage of buffers and whether or not they
	// are configured to drop on overflow.
	//
	// Example usage:
	// 	 metrics.BufferSize.WithLabelValues(b.String(), "true").Set(float64(capacity)/float64(maxCapacity))
	BufferSize *lazy.GaugeVec
}

var metricsByNamespace sync.Map

func New(namespace string) *Metrics {
	if existing, ok := metricsByNamespace.Load(namespace); ok {
		return existing.(*Metrics)
	}

	created := &Metrics{
		BufferState: lazy.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "buffer",
				Name:      "state_total",
				Help:      "Number of buffer inserts by name and state",
			},
			[]string{"name", "state"},
		),
		BufferSize: lazy.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: "buffer",
				Name:      "size_percentage",
				Help:      "Percentage of buffered fill capacity between 0.0 and 1.0",
			},
			[]string{"name", "drop"},
		),
	}

	actual, _ := metricsByNamespace.LoadOrStore(namespace, created)
	return actual.(*Metrics)
}

// ForBuffer binds the metric labels used by a buffer. It is nil-safe so the
// caller can use the returned value when metrics are disabled.
func (m *Metrics) ForBuffer(name string, drop bool) *BufferMetrics {
	if m == nil {
		return nil
	}

	dropLabel := "false"
	if drop {
		dropLabel = "true"
	}

	return &BufferMetrics{
		buffered: m.BufferState.WithLabelValues(name, "buffered"),
		dropped:  m.BufferState.WithLabelValues(name, "dropped"),
		closed:   m.BufferState.WithLabelValues(name, "closed"),
		size:     m.BufferSize.WithLabelValues(name, dropLabel),
	}
}

func (m *BufferMetrics) Buffered() {
	if m != nil {
		m.buffered.Inc()
	}
}

func (m *BufferMetrics) Dropped() {
	if m != nil {
		m.dropped.Inc()
	}
}

// Closed records an attempted write after the buffer has begun closing.
func (m *BufferMetrics) Closed() {
	if m != nil {
		m.closed.Inc()
	}
}

func (m *BufferMetrics) SetFillRatio(value float64) {
	if m != nil {
		m.size.Set(value)
	}
}
