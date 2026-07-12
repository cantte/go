package server

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/cantte/go/logger"
	"github.com/gin-gonic/gin"
)

func WithLogging() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path

		ctx, event := logger.StartWideEvent(c,
			fmt.Sprintf("%s %s", c.Request.Method, path),
		)

		defer event.End()

		c.Request = c.Request.WithContext(ctx)
		c.Next()

		if len(c.Errors) > 0 {
			for _, err := range c.Errors {
				event.SetError(err)
			}
		}

		requestID := c.GetString("request_id")
		event.Set(slog.Group("http",
			slog.String("request_id", requestID),
			slog.String("method", c.Request.Method),
			slog.String("path", path),
			slog.String("host", c.Request.Host),
			slog.String("user_agent", c.Request.UserAgent()),
			slog.String("ip_address", c.ClientIP()),
			slog.Int("status_code", c.Writer.Status()),
			slog.Int64("latency_ms", time.Since(start).Milliseconds()),
		))

	}
}
