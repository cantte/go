package assert

import (
	"errors"
)

// NotEmpty asserts that a string, slice, or map is not empty (has non-zero length).
// If the value is empty, it returns an error tagged with ASSERTION_FAILED.
//
// Example:
//
//	// Validate required input
//	if err := assert.NotEmpty(request.IDs, "At least one ID must be provided"); err != nil {
//	    return err
//	}
func NotEmpty[T ~string | ~[]any | ~[]string | ~map[any]any | []byte](value T, message ...string) error {
	if len(value) == 0 {
		errorMsg := "value is empty"
		if len(message) > 0 {
			errorMsg = message[0]
		}
		return errors.New(errorMsg)
	}
	return nil
}
