// Package money represents monetary amounts as integer minor units.
// Amounts are parsed from their exact decimal text; floating point is never used.
package money

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Amount is a monetary value in minor units (1/100 of the major unit).
type Amount int64

// Parsing errors.
var (
	ErrInvalidNumber   = errors.New("not a plain decimal number")
	ErrTooManyDecimals = errors.New("more than 2 decimal places")
	ErrOverflow        = errors.New("amount out of range")
)

const minorPerMajor = 100

// ParseAmount converts a JSON number such as "9500000.50" into minor units.
// Exponent notation and more than two decimal places are rejected.
func ParseAmount(n json.Number) (Amount, error) {
	s := string(n)
	negative := strings.HasPrefix(s, "-")
	if negative {
		s = s[1:]
	}

	whole, frac, hasDot := strings.Cut(s, ".")
	if !isDigits(whole) || (hasDot && !isDigits(frac)) {
		return 0, fmt.Errorf("%q: %w", n, ErrInvalidNumber)
	}
	if len(frac) > 2 {
		return 0, fmt.Errorf("%q: %w", n, ErrTooManyDecimals)
	}

	major, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%q: %w", n, ErrOverflow)
	}
	minor := int64(0)
	if frac != "" {
		minor, _ = strconv.ParseInt((frac + "0")[:2], 10, 64) // digits validated above
	}
	if major > (math.MaxInt64-minor)/minorPerMajor {
		return 0, fmt.Errorf("%q: %w", n, ErrOverflow)
	}

	total := major*minorPerMajor + minor
	if negative {
		total = -total
	}
	return Amount(total), nil
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// IsWhole reports whether the amount has no fractional minor units.
func (a Amount) IsWhole() bool { return a%minorPerMajor == 0 }

// IsMultipleOf reports whether the amount is a multiple of the given number of major units.
func (a Amount) IsMultipleOf(units int64) bool { return int64(a)%(units*minorPerMajor) == 0 }

// String renders the amount with exactly two decimals, e.g. "15000.00",
// suitable as a NUMERIC query parameter.
func (a Amount) String() string {
	v := int64(a)
	sign := ""
	if v < 0 {
		sign = "-"
		v = -v
	}
	return fmt.Sprintf("%s%d.%02d", sign, v/minorPerMajor, v%minorPerMajor)
}
