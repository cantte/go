package assert

import (
	"errors"
)

// Greater asserts that value 'a' is greater than value 'b'.
// If 'a' is not greater than 'b', it returns an error tagged with ASSERTION_FAILED.
//
// Example:
//
//	// Validate minimum balance
//	if err := assert.Greater(account.Balance, minimumRequired, "Account balance must exceed minimum"); err != nil {
//	    return err
//	}
func Greater[T ~int | ~int32 | ~int64 | ~float32 | ~float64 | ~uint | ~uint32 | ~uint64](a, b T, message ...string) error {
	if a > b {
		return nil
	}
	errorMsg := "value is not greater"
	if len(message) > 0 {
		errorMsg = message[0]
	}
	return errors.New(errorMsg)
}
