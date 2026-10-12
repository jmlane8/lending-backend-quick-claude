package money

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestParseAmount(t *testing.T) {
	tests := []struct {
		in      string
		want    Amount
		wantErr error
	}{
		{"15000", 1500000, nil},
		{"9500000.50", 950000050, nil},
		{"9500000.5", 950000050, nil},
		{"0", 0, nil},
		{"0.5", 50, nil},
		{"0.05", 5, nil},
		{"-1.25", -125, nil},
		{"1.005", 0, ErrTooManyDecimals},
		{"1e3", 0, ErrInvalidNumber},
		{"1E3", 0, ErrInvalidNumber},
		{"", 0, ErrInvalidNumber},
		{"abc", 0, ErrInvalidNumber},
		{"1.", 0, ErrInvalidNumber},
		{".5", 0, ErrInvalidNumber},
		{"+5", 0, ErrInvalidNumber},
		{"92233720368547758.07", 9223372036854775807, nil},
		{"92233720368547758.08", 0, ErrOverflow},
		{"92233720368547759", 0, ErrOverflow},
		{"99999999999999999999", 0, ErrOverflow},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseAmount(json.Number(tt.in))
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ParseAmount(%q) err = %v, want %v", tt.in, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseAmount(%q) unexpected err: %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("ParseAmount(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestAmountIsWhole(t *testing.T) {
	if !Amount(1500000).IsWhole() {
		t.Error("15000.00 should be whole")
	}
	if Amount(100050).IsWhole() {
		t.Error("1000.50 should not be whole")
	}
}

func TestAmountIsMultipleOf(t *testing.T) {
	tests := []struct {
		a     Amount
		units int64
		want  bool
	}{
		{1500000, 100, true},  // 15000
		{10000, 100, true},    // 100
		{1505000, 100, false}, // 15050
		{100050, 100, false},  // 1000.50
		{0, 100, true},
	}
	for _, tt := range tests {
		if got := tt.a.IsMultipleOf(tt.units); got != tt.want {
			t.Errorf("Amount(%d).IsMultipleOf(%d) = %v, want %v", tt.a, tt.units, got, tt.want)
		}
	}
}

func TestAmountString(t *testing.T) {
	tests := []struct {
		a    Amount
		want string
	}{
		{1500000, "15000.00"},
		{950000050, "9500000.50"},
		{5, "0.05"},
		{0, "0.00"},
		{-125, "-1.25"},
	}
	for _, tt := range tests {
		if got := tt.a.String(); got != tt.want {
			t.Errorf("Amount(%d).String() = %q, want %q", tt.a, got, tt.want)
		}
	}
}
