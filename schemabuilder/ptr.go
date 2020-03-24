package schemabuilder

import "time"

// BoolPtr transforms a bool into a *bool
func BoolPtr(i bool) *bool {
	return &i
}

// DurationPtr transforms a time.Duration into a *time.Duration
func DurationPtr(i time.Duration) *time.Duration {
	return &i
}

// Float64Ptr transforms a float64 into a *float64
func Float64Ptr(i float64) *float64 {
	return &i
}

// Float32Ptr transforms a float32 into a *float32
func Float32Ptr(i float32) *float32 {
	return &i
}

// IntPtr transforms an int into an *int
func IntPtr(i int) *int {
	return &i
}

// Int8Ptr transforms an int16 into an *int
func Int8Ptr(i int8) *int8 {
	return &i
}

// Int16Ptr transforms an int16 into an *int16
func Int16Ptr(i int16) *int16 {
	return &i
}

// Int32Ptr transforms an int32 into an *int32
func Int32Ptr(i int32) *int32 {
	return &i
}

// Int64Ptr transforms an int64 into an *int64
func Int64Ptr(i int64) *int64 {
	return &i
}

// StrPtr transforms a string into a *string
func StrPtr(i string) *string {
	return &i
}

// UInt8Ptr transforms a uint8 into a *uint8
func UInt8Ptr(i uint8) *uint8 {
	return &i
}

// UInt32Ptr transforms a uint32 into a *uint32
func UInt32Ptr(i uint32) *uint32 {
	return &i
}

// UInt16Ptr transforms an uint16 into an *uint16
func UInt16Ptr(i int16) *int16 {
	return &i
}

// UInt64Ptr transforms an uint64 into an *uint64
func UInt64Ptr(i int64) *int64 {
	return &i
}
