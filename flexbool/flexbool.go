// ABOUTME: FlexBool type that handles Amazon SP-API's inconsistent boolean
// ABOUTME: serialization — both native JSON bools and string-encoded bools.
package flexbool

import (
	"encoding/json"
	"fmt"
)

// FlexBool is a bool that can be unmarshalled from either a native JSON boolean
// (true / false) or a JSON string ("true" / "false"). Amazon's SP-API specs
// declare many fields as boolean, but the live API sometimes returns them as
// strings. FlexBool always marshals back to a native JSON boolean.
type FlexBool bool

// UnmarshalJSON handles both native JSON boolean and string-encoded boolean values.
func (fb *FlexBool) UnmarshalJSON(data []byte) error {
	// Try native JSON boolean first
	var b bool
	if err := json.Unmarshal(data, &b); err == nil {
		*fb = FlexBool(b)
		return nil
	}

	// Try string-encoded boolean
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		switch s {
		case "true":
			*fb = true
			return nil
		case "false":
			*fb = false
			return nil
		default:
			return fmt.Errorf("FlexBool: cannot convert string %q to bool", s)
		}
	}

	return fmt.Errorf("FlexBool: cannot unmarshal %s", string(data))
}

// MarshalJSON serializes FlexBool as a native JSON boolean.
func (fb FlexBool) MarshalJSON() ([]byte, error) {
	return json.Marshal(bool(fb))
}

// PtrFlexBool returns a pointer to a FlexBool initialized from a bool value.
func PtrFlexBool(v bool) *FlexBool {
	fb := FlexBool(v)
	return &fb
}
