package scheduler

import (
	"testing"
	"time"
)

func TestNextFire(t *testing.T) {
	utc := time.UTC
	cfg := Config{Hour: 5, Minute: 30, Loc: utc}

	cases := []struct {
		name string
		from time.Time
		want time.Time
	}{
		{
			name: "before T -> today",
			from: time.Date(2026, 6, 20, 3, 0, 0, 0, utc),
			want: time.Date(2026, 6, 20, 5, 30, 0, 0, utc),
		},
		{
			name: "after T -> tomorrow",
			from: time.Date(2026, 6, 20, 6, 0, 0, 0, utc),
			want: time.Date(2026, 6, 21, 5, 30, 0, 0, utc),
		},
		{
			name: "exactly at T -> tomorrow",
			from: time.Date(2026, 6, 20, 5, 30, 0, 0, utc),
			want: time.Date(2026, 6, 21, 5, 30, 0, 0, utc),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := nextFire(tc.from, cfg)
			if !got.Equal(tc.want) {
				t.Errorf("nextFire(%v) = %v, want %v", tc.from, got, tc.want)
			}
		})
	}
}

func TestNextFirePreservesWallClockAcrossDST(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}
	cfg := Config{Hour: 5, Minute: 30, Loc: loc}
	from := time.Date(2026, 3, 7, 6, 0, 0, 0, loc)
	got := nextFire(from, cfg)
	if got.Hour() != 5 || got.Minute() != 30 || got.Day() != 8 {
		t.Errorf("nextFire across DST = %v, want 2026-03-08 05:30 local", got)
	}
}
