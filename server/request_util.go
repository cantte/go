package server

import (
	"github.com/cantte/go/codes"
	"github.com/cantte/go/fault"
	"github.com/gin-gonic/gin"
)

// BindBody binds the request body to the given struct.
// If it fails, an error is returned, that you can directly return from your handler.
func BindBody[T any](c *gin.Context) (T, error) {
	// nolint:exhaustruct
	var req T

	if err := c.ShouldBindJSON(&req); err != nil {
		return req, fault.Wrap(err,
			fault.Code(codes.ErrorsBadRequest),
			fault.Internal("invalid request body"),
			fault.Public("Los datos enviados son inválidos"),
		)
	}

	return req, nil
}
