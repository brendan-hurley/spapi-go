// ABOUTME: FlexTime type that handles Amazon SP-API's inconsistent time
// ABOUTME: serialization — both valid RFC 3339 timestamps and empty strings.
package flextime

import (
	"time"
)

// FlexTime wraps time.Time to handle JSON unmarshaling from both valid
// RFC 3339 timestamps and empty strings. Amazon's SP-API specs declare many
// fields as date-time, but the live API sometimes returns "" for optional
// time fields. FlexTime always marshals back to a standard RFC 3339 timestamp.
type FlexTime struct {
	time.Time
}

// UnmarshalJSON handles valid RFC 3339 timestamps, empty strings, and null.
// Empty strings result in a zero time (check with .IsZero()).
func (ft *FlexTime) UnmarshalJSON(data []byte) error {
	s := string(data)

	// Handle null — same as time.Time behavior (no-op, stays zero)
	if s == "null" {
		return nil
	}

	// Handle empty string — Amazon sends "" for optional time fields
	if s == `""` {
		ft.Time = time.Time{}
		return nil
	}

	// Delegate to time.Time's UnmarshalJSON for normal timestamps
	return ft.Time.UnmarshalJSON(data)
}

// MarshalJSON serializes FlexTime as an RFC 3339 timestamp.
func (ft FlexTime) MarshalJSON() ([]byte, error) {
	return ft.Time.MarshalJSON()
}

// PtrFlexTime returns a pointer to a FlexTime initialized from a time.Time value.
func PtrFlexTime(v time.Time) *FlexTime {
	return &FlexTime{Time: v}
}
