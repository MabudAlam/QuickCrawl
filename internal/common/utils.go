// Package common provides shared utilities and types used across all modules.
// It contains error types, URL validation, constants, and helper functions.
package common

// Ptr returns a pointer to the given value.
// This is a convenience function to reduce verbosity when creating pointers
// to literal values.
func Ptr[T any](value T) *T {
	return &value
}

// BoolPtr returns a pointer to the given boolean value.
// Convenience function for creating boolean pointers.
func BoolPtr(value bool) *bool {
	return &value
}

// IntPtr returns a pointer to the given integer value.
// Convenience function for creating integer pointers.
func IntPtr(value int) *int {
	return &value
}

// StringPtr returns a pointer to the given string value.
// Convenience function for creating string pointers.
func StringPtr(value string) *string {
	return &value
}

// MinInt returns the smaller of two integers.
func MinInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// MaxInt returns the larger of two integers.
func MaxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// MinFloat64 returns the smaller of two float64 values.
func MinFloat64(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// MaxFloat64 returns the larger of two float64 values.
func MaxFloat64(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
