package assert

import (
	"errors"
)

// False asserts that a boolean value is false.
// If the value is true, it returns an error tagged with ASSERTION_FAILED.
//
// Example:
//
//	// Safety check
//	if err := assert.False(isShuttingDown, "Cannot perform operation during shutdown"); err != nil {
//	    return err
//	}
func False(value bool, message ...string) error {
	if value {
		errorMsg := "expected false but got true"
		if len(message) > 0 {
			errorMsg = message[0]
		}
		return errors.New(errorMsg)
	}
	return nil
}
