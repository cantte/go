package assert

import (
	"errors"
	"strings"
)

// Contains asserts that a string contains a specific substring.
// If the string does not contain the substring, it returns an error tagged with ASSERTION_FAILED.
//
// Example:
//
//	// Validate email format
//	if err := assert.Contains(email, "@", "Email must contain @ symbol"); err != nil {
//	    return err
//	}
func Contains(s, substr string, message ...string) error {
	if !strings.Contains(s, substr) {
		errorMsg := "string does not contain substring"
		if len(message) > 0 {
			errorMsg = message[0]
		}
		return errors.New(errorMsg)
	}
	return nil
}
