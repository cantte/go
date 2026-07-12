package prometheus

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func New() (*gin.Engine, error) {
	r := gin.New()

	// Register the Prometheus metrics handler at the /metrics endpoint
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	return r, nil
}

// NewWithRegistry creates a server that exposes metrics from a custom
// prometheus.Registry at the /metrics endpoint.
func NewWithRegistry(reg *prometheus.Registry) (*http.Server, error) {
	if reg == nil {
		return nil, fmt.Errorf("prometheus: nil registry")
	}

	r := gin.New()

	h := promhttp.HandlerFor(reg, promhttp.HandlerOpts{})

	// Register the Prometheus metrics handler at the /metrics endpoint
	r.GET("/metrics", gin.WrapH(h))

	srv := &http.Server{
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	return srv, nil
}
