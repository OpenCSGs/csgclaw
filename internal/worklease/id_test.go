package worklease

import "testing"

func TestNewIDIsValid(t *testing.T) {
	if id := NewID(); !ValidID(id) {
		t.Fatalf("NewID() = %q, want a valid canonical UUID", id)
	}
}

func TestValidID(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "canonical", value: "550e8400-e29b-41d4-a716-446655440000", want: true},
		{name: "uppercase", value: "550E8400-E29B-41D4-A716-446655440000", want: true},
		{name: "compact", value: "550e8400e29b41d4a716446655440000", want: false},
		{name: "braced", value: "{550e8400-e29b-41d4-a716-446655440000}", want: false},
		{name: "urn", value: "urn:uuid:550e8400-e29b-41d4-a716-446655440000", want: false},
		{name: "invalid hex", value: "550e8400-e29b-41d4-a716-44665544000g", want: false},
		{name: "invalid separator", value: "550e8400xe29b-41d4-a716-446655440000", want: false},
		{name: "empty", value: "", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ValidID(test.value); got != test.want {
				t.Fatalf("ValidID(%q) = %t, want %t", test.value, got, test.want)
			}
		})
	}
}
