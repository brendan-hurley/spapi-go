// ABOUTME: Tests for the FlexBool type that handles Amazon SP-API's inconsistent
// ABOUTME: boolean serialization (both native JSON bools and string-encoded bools).
package flexbool

import (
	"encoding/json"
	"testing"
)

func TestFlexBool_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    FlexBool
		wantErr bool
	}{
		{"native true", `true`, FlexBool(true), false},
		{"native false", `false`, FlexBool(false), false},
		{"string true", `"true"`, FlexBool(true), false},
		{"string false", `"false"`, FlexBool(false), false},
		{"invalid string", `"yes"`, FlexBool(false), true},
		{"empty string", `""`, FlexBool(false), true},
		{"number 1", `1`, FlexBool(false), true},
		{"number 0", `0`, FlexBool(false), true},
		{"object", `{}`, FlexBool(false), true},
		{"array", `[]`, FlexBool(false), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var fb FlexBool
			err := fb.UnmarshalJSON([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && fb != tt.want {
				t.Errorf("UnmarshalJSON() = %v, want %v", fb, tt.want)
			}
		})
	}
}

func TestFlexBool_MarshalJSON(t *testing.T) {
	tests := []struct {
		name string
		fb   FlexBool
		want string
	}{
		{"true", FlexBool(true), "true"},
		{"false", FlexBool(false), "false"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.fb.MarshalJSON()
			if err != nil {
				t.Fatalf("MarshalJSON() error = %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("MarshalJSON() = %s, want %s", string(got), tt.want)
			}
		})
	}
}

func TestFlexBool_InStruct_PointerField(t *testing.T) {
	type testStruct struct {
		Flag *FlexBool `json:"flag,omitempty"`
	}

	tests := []struct {
		name string
		json string
		want bool
	}{
		{"bool true", `{"flag": true}`, true},
		{"bool false", `{"flag": false}`, false},
		{"string true", `{"flag": "true"}`, true},
		{"string false", `{"flag": "false"}`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s testStruct
			if err := json.Unmarshal([]byte(tt.json), &s); err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}
			if s.Flag == nil {
				t.Fatal("Flag is nil")
			}
			if bool(*s.Flag) != tt.want {
				t.Errorf("Flag = %v, want %v", *s.Flag, tt.want)
			}
		})
	}
}

func TestFlexBool_InStruct_NonPointerField(t *testing.T) {
	type testStruct struct {
		Flag FlexBool `json:"flag"`
	}

	tests := []struct {
		name string
		json string
		want bool
	}{
		{"bool true", `{"flag": true}`, true},
		{"bool false", `{"flag": false}`, false},
		{"string true", `{"flag": "true"}`, true},
		{"string false", `{"flag": "false"}`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s testStruct
			if err := json.Unmarshal([]byte(tt.json), &s); err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}
			if bool(s.Flag) != tt.want {
				t.Errorf("Flag = %v, want %v", s.Flag, tt.want)
			}
		})
	}
}

func TestFlexBool_InStruct_OmittedField(t *testing.T) {
	type testStruct struct {
		Flag *FlexBool `json:"flag,omitempty"`
	}

	var s testStruct
	if err := json.Unmarshal([]byte(`{}`), &s); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if s.Flag != nil {
		t.Errorf("expected nil for omitted field, got %v", *s.Flag)
	}
}

func TestFlexBool_NullField(t *testing.T) {
	type testStruct struct {
		Flag *FlexBool `json:"flag"`
	}

	var s testStruct
	if err := json.Unmarshal([]byte(`{"flag": null}`), &s); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if s.Flag != nil {
		t.Errorf("expected nil for null field, got %v", *s.Flag)
	}
}

func TestFlexBool_RoundTrip(t *testing.T) {
	type testStruct struct {
		A *FlexBool `json:"a,omitempty"`
		B *FlexBool `json:"b,omitempty"`
		C *FlexBool `json:"c,omitempty"`
	}

	// Unmarshal from mixed formats
	input := `{"a": true, "b": "false", "c": null}`
	var s testStruct
	if err := json.Unmarshal([]byte(input), &s); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	// Marshal back — should produce native JSON booleans
	out, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	expected := `{"a":true,"b":false}`
	if string(out) != expected {
		t.Errorf("Marshal() = %s, want %s", string(out), expected)
	}
}

func TestFlexBool_BoolConversion(t *testing.T) {
	fb := FlexBool(true)
	if !bool(fb) {
		t.Error("bool(FlexBool(true)) should be true")
	}

	fb = FlexBool(false)
	if bool(fb) {
		t.Error("bool(FlexBool(false)) should be false")
	}
}

func TestFlexBool_CanUseInConditional(t *testing.T) {
	fb := FlexBool(true)
	if !fb {
		t.Error("FlexBool(true) should be truthy in conditional")
	}

	fb = FlexBool(false)
	if fb {
		t.Error("FlexBool(false) should be falsy in conditional")
	}
}

func TestPtrFlexBool(t *testing.T) {
	p := PtrFlexBool(true)
	if p == nil {
		t.Fatal("PtrFlexBool returned nil")
	}
	if *p != FlexBool(true) {
		t.Errorf("PtrFlexBool(true) = %v, want true", *p)
	}

	p = PtrFlexBool(false)
	if *p != FlexBool(false) {
		t.Errorf("PtrFlexBool(false) = %v, want false", *p)
	}
}
