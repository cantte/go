package db

import "testing"

func TestNewReplicaMetricsOptions(t *testing.T) {
	t.Parallel()

	defaultReplica := NewReplica(nil, "default")
	if defaultReplica.metrics == nil {
		t.Fatal("metrics should be enabled by default")
	}

	disabledReplica := NewReplica(nil, "disabled", WithMetrics(false))
	if disabledReplica.metrics != nil {
		t.Fatal("metrics should be disabled")
	}

	first := NewReplica(nil, "first", WithMetricsNamespace("project"))
	second := NewReplica(nil, "second", WithMetricsNamespace("project"))
	if first.metrics != second.metrics {
		t.Fatal("replicas with the same namespace should share metrics")
	}
}

func TestNewReplicaOptionsAreAppliedInOrder(t *testing.T) {
	t.Parallel()

	replica := NewReplica(nil, "replica", WithMetrics(false), WithMetrics(true))
	if replica.metrics == nil {
		t.Fatal("last metrics option should win")
	}
}
