package httpx

import (
	"encoding/json"
	"testing"
	"time"
)

func withDisplayLocation(t *testing.T, name string) {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	SetDisplayLocation(loc)
	t.Cleanup(func() { SetDisplayLocation(time.UTC) })
}

func TestTimestampMarshal(t *testing.T) {
	instant := time.Date(2026, 5, 5, 7, 30, 0, 123456789, time.UTC)

	withDisplayLocation(t, "Asia/Jakarta")
	b, err := json.Marshal(Timestamp{instant})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(b), `"2026-05-05T14:30:00+07:00"`; got != want {
		t.Errorf("Jakarta: got %s, want %s", got, want)
	}

	SetDisplayLocation(time.UTC)
	b, err = json.Marshal(Timestamp{instant})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(b), `"2026-05-05T07:30:00+00:00"`; got != want {
		t.Errorf("UTC: got %s, want %s (never Z)", got, want)
	}
}

func TestTimestampUnmarshal(t *testing.T) {
	tests := []struct {
		in      string
		wantErr bool
	}{
		{`"2026-05-05T14:30:00+07:00"`, false},
		{`"2026-05-05T07:30:00+00:00"`, false},
		{`"2026-05-05T07:30:00Z"`, true},
		{`"2026-05-05T14:30:00.5+07:00"`, true},
		{`"2026-05-05T14:30:00"`, true},
		{`"2026-05-05"`, true},
		{`123`, true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			var ts Timestamp
			err := json.Unmarshal([]byte(tt.in), &ts)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Unmarshal(%s) err = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
		})
	}

	var ts Timestamp
	if err := json.Unmarshal([]byte(`"2026-05-05T14:30:00+07:00"`), &ts); err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 5, 5, 7, 30, 0, 0, time.UTC); !ts.Equal(want) {
		t.Errorf("parsed instant = %v, want %v", ts.Time, want)
	}
}

func TestParseDate(t *testing.T) {
	tests := []struct {
		in      string
		wantErr bool
	}{
		{"1990-04-17", false},
		{"2024-02-29", false},
		{"2001-02-30", true},
		{"1990-4-17", true},
		{"17-04-1990", true},
		{"1990-04-17T00:00:00+07:00", true},
		{"", true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			d, err := ParseDate(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseDate(%q) err = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if err == nil && d.String() != tt.in {
				t.Errorf("round trip = %q, want %q", d.String(), tt.in)
			}
		})
	}
}

func TestDateJSON(t *testing.T) {
	var d Date
	if err := json.Unmarshal([]byte(`"1990-04-17"`), &d); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `"1990-04-17"` {
		t.Errorf("got %s", b)
	}
	if err := json.Unmarshal([]byte(`"2001-02-30"`), &d); err == nil {
		t.Error("expected error for 2001-02-30")
	}
}
