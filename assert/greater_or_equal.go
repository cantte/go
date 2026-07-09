package assert

import (
	"errors"
)

// GreaterOrEqual asserts that value 'a' is greater or equal compared to value 'b'.
// If 'a' is not greater or equal than 'b', it returns an error tagged with ASSERTION_FAILED.
//
// Example:
//
//	// Validate minimum balance
//	if err := assert.GreaterOrEqual(account.Balance, minimumRequired, "Account balance must meet minimum"); err != nil {
//	    return err
//	}
func GreaterOrEqual[T ~int | ~int32 | ~int64 | ~float32 | ~float64 | ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64](a, b T, message ...string) error {
	if a >= b {
		return nil
	}

	errorMsg := "value is not greater or equal"
	if len(message) > 0 {
		errorMsg = message[0]
	}
	return errors.New(errorMsg)
}
