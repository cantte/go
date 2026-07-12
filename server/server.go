package server

import "github.com/gin-gonic/gin"

// RegisterRoute registers a single [Route] on the given router group.
// It delegates to [gin.IRoutes.Handle] using the method and path reported by
// the route itself, so each handler is self-describing and no central routing
// table is needed.
func RegisterRoute(g gin.IRoutes, route Route) {
	g.Handle(route.Method(), route.Path(), ToGinHandler(route))
}

func ToGinHandler(h Handler) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := h.Handle(c); err != nil {
			_ = c.Error(err)
			return
		}
	}
}
