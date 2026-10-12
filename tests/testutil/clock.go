package testutil

import "time"

// FixedNow is the instant returned by FixedClock: 2026-10-12 10:00:00 Asia/Jakarta.
var FixedNow = time.Date(2026, 10, 12, 3, 0, 0, 0, time.UTC)

// FixedClock returns a clock that always reports FixedNow.
func FixedClock() func() time.Time { return func() time.Time { return FixedNow } }
