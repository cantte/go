package server

import (
	"github.com/cantte/go/uid"
	"github.com/gin-gonic/gin"
)

func WithRequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = uid.New(uid.RequestPrefix)
		}
		c.Set("request_id", requestID)
		c.Header("X-Request-ID", requestID)
		c.Next()
	}
}
