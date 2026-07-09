package assert

import (
	"errors"
)

// True asserts that a boolean value is true.
// If the value is false, it returns an error tagged with ASSERTION_FAILED.
//
// Example:
//
//	// Verify a precondition
//	if err := assert.True(len(items) > 0, "items cannot be empty"); err != nil {
//	    return err
//	}
func True(value bool, message ...string) error {
	if !value {
		errorMsg := "expected true but got false"
		if len(message) > 0 {
			errorMsg = message[0]
		}
		return errors.New(errorMsg)
	}
	return nil
}
