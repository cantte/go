package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigure(t *testing.T) {
	assert.Error(t, Configure(""))

	assert.NoError(t, Configure("metrics_test"))
	assert.Same(t, New("metrics_test"), Configured())

	assert.NoError(t, Configure("metrics_test"))
	assert.Error(t, Configure("another_application"))
}

func TestNewRequiresNamespace(t *testing.T) {
	assert.Panics(t, func() {
		New("")
	})
}
