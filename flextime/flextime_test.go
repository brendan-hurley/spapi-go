// ABOUTME: Tests for the FlexTime type that handles Amazon SP-API's inconsistent
// ABOUTME: time serialization (both valid timestamps and empty strings).
package flextime

import (
	"encoding/json"
	"testing"
	"time"
)

func TestFlexTime_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantZero bool
		wantErr  bool
		wantYear int // 0 means don't check
	}{
		{"valid RFC 3339 UTC", `"2024-01-15T10:30:00Z"`, false, false, 2024},
		{"valid RFC 3339 offset", `"2024-01-15T10:30:00+05:00"`, false, false, 2024},
		{"valid RFC 3339 negative offset", `"2024-06-01T08:00:00-07:00"`, false, false, 2024},
		{"empty string", `""`, true, false, 0},
		{"null", `null`, true, false, 0},
		{"invalid format", `"not-a-date"`, false, true, 0},
		{"number", `12345`, false, true, 0},
		{"object", `{}`, false, true, 0},
		{"partial date", `"2024-01-15"`, false, true, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ft FlexTime
			err := ft.UnmarshalJSON([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantZero && !ft.Time.IsZero() {
				t.Errorf("expected zero time, got %v", ft.Time)
			}
			if tt.wantYear > 0 && ft.Time.Year() != tt.wantYear {
				t.Errorf("expected year %d, got %d", tt.wantYear, ft.Time.Year())
			}
		})
	}
}

func TestFlexTime_MarshalJSON(t *testing.T) {
	ts := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	ft := FlexTime{Time: ts}

	got, err := ft.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}

	want := `"2024-01-15T10:30:00Z"`
	if string(got) != want {
		t.Errorf("MarshalJSON() = %s, want %s", string(got), want)
	}
}

func TestFlexTime_MarshalJSON_Zero(t *testing.T) {
	ft := FlexTime{}
	got, err := ft.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}

	// Zero time marshals to Go's default representation
	var zero time.Time
	want, _ := zero.MarshalJSON()
	if string(got) != string(want) {
		t.Errorf("MarshalJSON() = %s, want %s", string(got), string(want))
	}
}

func TestFlexTime_InStruct_PointerField(t *testing.T) {
	type testStruct struct {
		Updated *FlexTime `json:"updated,omitempty"`
	}

	tests := []struct {
		name     string
		json     string
		wantNil  bool
		wantZero bool
		wantYear int
	}{
		{"valid timestamp", `{"updated": "2024-01-15T10:30:00Z"}`, false, false, 2024},
		{"empty string", `{"updated": ""}`, false, true, 0},
		{"null", `{"updated": null}`, true, false, 0},
		{"absent field", `{}`, true, false, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s testStruct
			if err := json.Unmarshal([]byte(tt.json), &s); err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}
			if tt.wantNil {
				if s.Updated != nil {
					t.Errorf("expected nil, got %v", s.Updated.Time)
				}
				return
			}
			if s.Updated == nil {
				t.Fatal("Updated is nil")
			}
			if tt.wantZero && !s.Updated.Time.IsZero() {
				t.Errorf("expected zero time, got %v", s.Updated.Time)
			}
			if tt.wantYear > 0 && s.Updated.Time.Year() != tt.wantYear {
				t.Errorf("expected year %d, got %d", tt.wantYear, s.Updated.Time.Year())
			}
		})
	}
}

func TestFlexTime_InStruct_NonPointerField(t *testing.T) {
	type testStruct struct {
		Created FlexTime `json:"created"`
	}

	tests := []struct {
		name     string
		json     string
		wantZero bool
		wantYear int
	}{
		{"valid timestamp", `{"created": "2024-01-15T10:30:00Z"}`, false, 2024},
		{"empty string", `{"created": ""}`, true, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s testStruct
			if err := json.Unmarshal([]byte(tt.json), &s); err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}
			if tt.wantZero && !s.Created.Time.IsZero() {
				t.Errorf("expected zero time, got %v", s.Created.Time)
			}
			if tt.wantYear > 0 && s.Created.Time.Year() != tt.wantYear {
				t.Errorf("expected year %d, got %d", tt.wantYear, s.Created.Time.Year())
			}
		})
	}
}

func TestFlexTime_RoundTrip(t *testing.T) {
	type testStruct struct {
		A *FlexTime `json:"a,omitempty"`
		B *FlexTime `json:"b,omitempty"`
		C *FlexTime `json:"c,omitempty"`
	}

	// Unmarshal: valid time, empty string, null
	input := `{"a": "2024-01-15T10:30:00Z", "b": "", "c": null}`
	var s testStruct
	if err := json.Unmarshal([]byte(input), &s); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if s.A == nil || s.A.Time.Year() != 2024 {
		t.Error("field 'a' should be 2024")
	}
	if s.B == nil || !s.B.Time.IsZero() {
		t.Error("field 'b' should be zero time (from empty string)")
	}
	if s.C != nil {
		t.Error("field 'c' should be nil (from null)")
	}

	// Marshal back — 'a' has a real time, 'b' has zero time, 'c' is nil (omitted)
	out, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	// Should contain 'a' and 'b' but not 'c'
	var result map[string]interface{}
	json.Unmarshal(out, &result)
	if _, ok := result["a"]; !ok {
		t.Error("expected 'a' in output")
	}
	if _, ok := result["b"]; !ok {
		t.Error("expected 'b' in output (zero time, but pointer is non-nil)")
	}
	if _, ok := result["c"]; ok {
		t.Error("expected 'c' to be omitted")
	}
}

func TestFlexTime_TimeAccess(t *testing.T) {
	ts := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	ft := FlexTime{Time: ts}

	// FlexTime embeds time.Time, so all methods are available
	if ft.Year() != 2024 {
		t.Errorf("Year() = %d, want 2024", ft.Year())
	}
	if ft.Month() != time.June {
		t.Errorf("Month() = %v, want June", ft.Month())
	}
	if ft.Day() != 15 {
		t.Errorf("Day() = %d, want 15", ft.Day())
	}

	// Direct .Time field access
	if ft.Time != ts {
		t.Errorf("Time = %v, want %v", ft.Time, ts)
	}
}

func TestPtrFlexTime(t *testing.T) {
	ts := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	p := PtrFlexTime(ts)
	if p == nil {
		t.Fatal("PtrFlexTime returned nil")
	}
	if p.Time != ts {
		t.Errorf("PtrFlexTime().Time = %v, want %v", p.Time, ts)
	}
}
