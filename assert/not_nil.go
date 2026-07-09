package assert

import (
	"errors"
)

// NotNil asserts that the provided value is not nil.
// If the value is nil, it returns an error tagged with ASSERTION_FAILED.
//
// Example:
//
//	if err := assert.NotNil(user, "User must be provided"); err != nil {
//	    return err
//	}
func NotNil(t any, message ...string) error {
	if t == nil {
		errorMsg := "expected not nil"
		if len(message) > 0 {
			errorMsg = message[0]
		}
		return errors.New(errorMsg)
	}
	return nil
}
