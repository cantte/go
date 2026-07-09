package assert

import (
	"errors"
)

// LessOrEqual asserts that value 'a' is less or equal compared to value 'b'.
// If 'a' is not less or equal than 'b', it returns an error tagged with ASSERTION_FAILED.
//
// Example:
//
//	// Validate maximum limit
//	if err := assert.LessOrEqual(requestCount, maxAllowed, "Request count must not exceed maximum"); err != nil {
//	    return err
//	}
func LessOrEqual[T ~int | ~int32 | ~int64 | ~float32 | ~float64](a, b T, message ...string) error {
	if a <= b {
		return nil
	}

	errorMsg := "value is not less or equal"
	if len(message) > 0 {
		errorMsg = message[0]
	}
	return errors.New(errorMsg)
}
