package assert

import (
	"errors"
)

// NotEqual asserts that two values of the same comparable type are not equal.
// If the values are equal, it returns an error tagged with ASSERTION_FAILED.
//
// Example:
//
//	// Verify values are different
//	if err := assert.NotEqual(userID, adminID, "User should not be admin"); err != nil {
//	    return err
//	}
func NotEqual[T comparable](a T, b T, message ...string) error {
	if a == b {
		errorMsg := "expected not equal"
		if len(message) > 0 {
			errorMsg = message[0]
		}
		return errors.New(errorMsg)
	}
	return nil
}
