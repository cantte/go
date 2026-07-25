package metrics

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfigureIsIdempotentForSameNamespace(t *testing.T) {
	require.NoError(t, Configure("circuitbreaker_test"))
	require.NoError(t, Configure("circuitbreaker_test"))
	require.Error(t, Configure("different_namespace"))
}

func TestRecordRequestWithoutConfiguration(t *testing.T) {
	var unconfigured *Metrics
	require.NotPanics(t, func() {
		unconfigured.RecordRequest("service", "closed")
		unconfigured.RecordError("service", "tripped")
	})
}

func TestNewReusesMetricsForSameNamespace(t *testing.T) {
	// The lazy collectors register with prometheus on first use, so two
	// distinct Metrics for one namespace would panic with a duplicate
	// registration as soon as both were used.
	first := New("shared_namespace")
	second := New("shared_namespace")
	require.Same(t, first, second)

	require.NotPanics(t, func() {
		first.RecordRequest("service", "closed")
		second.RecordRequest("service", "closed")
		first.RecordError("service", "tripped")
		second.RecordError("service", "too_many_requests")
	})
}
