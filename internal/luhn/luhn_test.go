package luhn

import "testing"

func TestIsValid(t *testing.T) {
	tests := []struct {
		name   string
		number string
		want   bool
	}{
		{name: "valid short", number: "79927398713", want: true},
		{name: "valid from specification", number: "12345678903", want: true},
		{name: "valid another", number: "2377225624", want: true},
		{name: "single zero", number: "0", want: true},
		{name: "invalid checksum", number: "12345678901", want: false},
		{name: "empty", number: "", want: false},
		{name: "letters", number: "12345abc", want: false},
		{name: "spaces", number: "1234 5678 903", want: false},
		{name: "negative", number: "-12345678903", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValid(tt.number); got != tt.want {
				t.Errorf("IsValid(%q) = %v, want %v", tt.number, got, tt.want)
			}
		})
	}
}
