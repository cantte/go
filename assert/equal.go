package assert

import (
	"errors"
)

// Equal asserts that two values of the same comparable type are equal.
// If the values are not equal, it returns an error tagged with ASSERTION_FAILED.
//
// Example:
//
//	// Verify a calculation result
//	if err := assert.Equal(calculateTotal(), 100.0, "Total should be 100.0"); err != nil {
//	    return err
//	}
func Equal[T comparable](a T, b T, message ...string) error {
	if a != b {
		errorMsg := "expected equal"
		if len(message) > 0 {
			errorMsg = message[0]
		}
		return errors.New(errorMsg)
	}
	return nil
}
