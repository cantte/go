package server

import (
	"github.com/gin-gonic/gin"
)

// Handler is the core interface for an HTTP endpoint. Every route handler
// must implement Handle to process a request and write a response.
type Handler interface {
	Handle(ctx *gin.Context) error
}

// HandleFunc is a function type that satisfies [Handler], allowing plain
// functions to be used as handlers without defining a struct.
type HandleFunc func(ctx *gin.Context) error
