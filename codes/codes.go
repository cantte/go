package codes

// URN is a string type for error code constants
type URN string

const (
	// ServiceUnavailable indicates a service is temporarily unavailable.
	AppErrorsInternalServiceUnavailable URN = "err:application:service_unavailable"
)
