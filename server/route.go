package server

// Route extends [Handler] with the HTTP method and URL path that the handler
// should be mounted on. Implementing this interface allows each handler to be
// self-describing and registered via [RegisterRoute] without a central routing
// table.
type Route interface {
	Handler

	// Method returns the HTTP method (e.g. "GET", "POST") for this route.
	Method() string
	// Path returns the URL path pattern (e.g. "/v1/tenants/:id") for this route.
	Path() string
}
