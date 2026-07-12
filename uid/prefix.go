package uid

// Prefix is a resource type identifier prepended to generated IDs.
type Prefix string

const (
	RequestPrefix Prefix = "req"
	TestPrefix    Prefix = "test" // for tests only
)
