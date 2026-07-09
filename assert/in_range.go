package assert

import (
	"errors"
)

// InRange asserts that a value is within a specified range (inclusive).
// If the value is outside the range, it returns an error tagged with ASSERTION_FAILED.
//
// Example:
//
//	// Validate age input
//	if err := assert.InRange(age, 18, 120, "Age must be between 18 and 120"); err != nil {
//	    return err
//	}
func InRange[T ~int | ~float64](value, minimum, maximum T, message ...string) error {
	if value < minimum || value > maximum {
		errorMsg := "value is out of range"
		if len(message) > 0 {
			errorMsg = message[0]
		}
		return errors.New(errorMsg)
	}
	return nil
}
