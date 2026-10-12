package httpx

import (
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"
)

// TimestampLayout is the only accepted API timestamp format (constitution Principle I):
// whole seconds and a numeric offset, never "Z".
const TimestampLayout = "2006-01-02T15:04:05-07:00"

// DateLayout is the format of calendar dates such as a birth date.
const DateLayout = "2006-01-02"

var displayLocation atomic.Pointer[time.Location]

func init() { displayLocation.Store(time.UTC) }

// SetDisplayLocation sets the timezone that API timestamps are rendered in.
// It is called once at startup with APP_TIMEZONE.
func SetDisplayLocation(loc *time.Location) { displayLocation.Store(loc) }

// DisplayLocation returns the timezone API timestamps are rendered in.
func DisplayLocation() *time.Location { return displayLocation.Load() }

// Timestamp is a time.Time that marshals in the API timestamp format.
type Timestamp struct{ time.Time }

// MarshalJSON renders the instant in the display location, truncated to seconds.
func (t Timestamp) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.In(DisplayLocation()).Truncate(time.Second).Format(TimestampLayout))
}

// UnmarshalJSON accepts only TimestampLayout. Go's parser tolerates fractional
// seconds, so the value must also round-trip exactly.
func (t *Timestamp) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("timestamp must be a string in format %s", TimestampLayout)
	}
	parsed, err := time.Parse(TimestampLayout, s)
	if err != nil || parsed.Format(TimestampLayout) != s {
		return fmt.Errorf("timestamp %q must use format %s", s, TimestampLayout)
	}
	t.Time = parsed
	return nil
}

// Date is a calendar date without time or timezone.
type Date struct{ t time.Time }

// ParseDate parses a strict YYYY-MM-DD date and rejects impossible dates such as 2001-02-30.
func ParseDate(s string) (Date, error) {
	t, err := time.Parse(DateLayout, s)
	if err != nil {
		return Date{}, fmt.Errorf("date %q must be a real calendar date in format YYYY-MM-DD", s)
	}
	return Date{t: t}, nil
}

// Time returns the date at midnight UTC.
func (d Date) Time() time.Time { return d.t }

// String renders the date as YYYY-MM-DD.
func (d Date) String() string { return d.t.Format(DateLayout) }

// MarshalJSON renders the date as "YYYY-MM-DD".
func (d Date) MarshalJSON() ([]byte, error) { return json.Marshal(d.String()) }

// UnmarshalJSON accepts only "YYYY-MM-DD".
func (d *Date) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("date must be a string in format YYYY-MM-DD")
	}
	parsed, err := ParseDate(s)
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}
