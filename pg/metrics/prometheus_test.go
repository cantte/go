package metrics

import "testing"

func TestNewSharesMetricsByNamespace(t *testing.T) {
	t.Parallel()

	first := New("shared_test_namespace")
	second := New("shared_test_namespace")
	if first != second {
		t.Fatal("New returned different metric sets for the same namespace")
	}
}

func TestNewSeparatesMetricsByNamespace(t *testing.T) {
	t.Parallel()

	first := New("first_test_namespace")
	second := New("second_test_namespace")
	if first == second {
		t.Fatal("New returned the same metric set for different namespaces")
	}
}
