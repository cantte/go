package db

import (
	"encoding/json"
	"fmt"
)

// UnmarshalNullableJSONTo unmarshals JSON data from database columns into Go types.
// It handles the common pattern where database queries return JSON as []byte that needs
// to be deserialized into structs, slices, or maps.
//
// The function accepts 'any' type because database drivers return interface{} for JSON columns,
// even though the underlying value is typically []byte.
//
// Returns:
//   - (T, nil) on successful unmarshal
//   - (zero, nil) if data is nil or empty []byte (these are valid null/empty states)
//   - (zero, error) if type assertion fails or JSON unmarshal fails
//
// Example usage:
//
//	settings, err := UnmarshalNullableJSONTo[Type](row.Type)
//	if err != nil {
//	    logger.Error("failed to unmarshal type", "error", err)
//	    return err
//	}
func UnmarshalNullableJSONTo[T any](data any) (T, error) {
	var zero T
	if data == nil {
		return zero, nil
	}

	var bytes []byte
	switch v := data.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return zero, fmt.Errorf("type assertion failed during unmarshal: expected []byte or string, got %T", data)
	}

	if len(bytes) == 0 {
		return zero, nil
	}

	var result T
	if err := json.Unmarshal(bytes, &result); err != nil {
		return zero, fmt.Errorf("json unmarshal failed: %w", err)
	}

	return result, nil
}
