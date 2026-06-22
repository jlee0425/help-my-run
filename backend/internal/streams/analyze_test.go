package streams

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestTimeInZone(t *testing.T) {
	zb := ZoneBounds{Z1Hi: 116, Z2Hi: 145, Z3Hi: 157.5, Z4Hi: 170}
	tests := []struct {
		name    string
		s       Series
		wantLen int
		wantSec [5]float64
		wantPct [5]float64
	}{
		{
			name:    "all aerobic Z2",
			s:       Series{T: []float64{0, 1, 2, 3}, HR: []float64{120, 130, 140, 144}, V: []float64{2, 2, 2, 2}, Dist: []float64{0, 2, 4, 6}},
			wantLen: 5,
			wantSec: [5]float64{0, 4, 0, 0, 0},
			wantPct: [5]float64{0, 100, 0, 0, 0},
		},
		{
			name:    "mixed Z1/Z2/Z5",
			s:       Series{T: []float64{0, 1, 2, 3}, HR: []float64{100, 130, 175, 175}, V: []float64{1, 1, 1, 1}, Dist: []float64{0, 1, 2, 3}},
			wantLen: 5,
			wantSec: [5]float64{1, 1, 0, 0, 2},
			wantPct: [5]float64{25, 25, 0, 0, 50},
		},
		{
			name:    "no HR -> empty slice",
			s:       Series{T: []float64{0, 1, 2}, HR: nil, V: []float64{1, 1, 1}, Dist: []float64{0, 1, 2}},
			wantLen: 0,
		},
		{
			name:    "boundary inclusive-low/exclusive-high",
			s:       Series{T: []float64{0, 1, 2}, HR: []float64{116, 145, 170}, V: []float64{1, 1, 1}, Dist: []float64{0, 1, 2}},
			wantLen: 5,
			wantSec: [5]float64{0, 1, 1, 0, 1},
			wantPct: [5]float64{0, 100.0 / 3, 100.0 / 3, 0, 100.0 / 3},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TimeInZone(tt.s, zb)
			if len(got) != tt.wantLen {
				t.Fatalf("len = %d, want %d (%+v)", len(got), tt.wantLen, got)
			}
			if tt.wantLen == 0 {
				return
			}
			for i, zt := range got {
				if zt.Zone != i+1 {
					t.Errorf("zone[%d].Zone = %d, want %d", i, zt.Zone, i+1)
				}
				if !approx(zt.Seconds, tt.wantSec[i]) {
					t.Errorf("zone %d seconds = %v, want %v", i+1, zt.Seconds, tt.wantSec[i])
				}
				if !approx(zt.Pct, tt.wantPct[i]) {
					t.Errorf("zone %d pct = %v, want %v", i+1, zt.Pct, tt.wantPct[i])
				}
			}
		})
	}
}

func TestComputeDecoupling(t *testing.T) {
	tests := []struct {
		name      string
		s         Series
		wantNil   bool
		wantPct   float64
		wantFirst float64
		wantSec   float64
	}{
		{
			name:      "clean drift: HR rises 2nd half, pace held -> positive decoupling",
			s:         Series{T: []float64{0, 1, 2, 3}, HR: []float64{100, 100, 125, 125}, V: []float64{2, 2, 2, 2}, Dist: []float64{0, 2, 4, 6}},
			wantPct:   20,
			wantFirst: 0.02,
			wantSec:   0.016,
		},
		{
			name:    "no HR -> all nil",
			s:       Series{T: []float64{0, 1, 2, 3}, HR: nil, V: []float64{2, 2, 2, 2}, Dist: []float64{0, 2, 4, 6}},
			wantNil: true,
		},
		{
			name:    "too short: <2 samples per half -> all nil",
			s:       Series{T: []float64{0, 1}, HR: []float64{100, 110}, V: []float64{2, 2}, Dist: []float64{0, 2}},
			wantNil: true,
		},
		{
			name:    "zero mean HR in a half -> all nil",
			s:       Series{T: []float64{0, 1, 2, 3}, HR: []float64{0, 0, 120, 120}, V: []float64{2, 2, 2, 2}, Dist: []float64{0, 2, 4, 6}},
			wantNil: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeDecoupling(tt.s)
			if tt.wantNil {
				if got.DecouplingPct != nil || got.PaHRFirst != nil || got.PaHRSecond != nil {
					t.Fatalf("want all nil, got %+v", got)
				}
				return
			}
			if got.DecouplingPct == nil || !approx(*got.DecouplingPct, tt.wantPct) {
				t.Errorf("decoupling_pct = %v, want %v", got.DecouplingPct, tt.wantPct)
			}
			if got.PaHRFirst == nil || !approx(*got.PaHRFirst, tt.wantFirst) {
				t.Errorf("pa_hr_first = %v, want %v", got.PaHRFirst, tt.wantFirst)
			}
			if got.PaHRSecond == nil || !approx(*got.PaHRSecond, tt.wantSec) {
				t.Errorf("pa_hr_second = %v, want %v", got.PaHRSecond, tt.wantSec)
			}
		})
	}
}

func TestAnalyzeAggregate(t *testing.T) {
	zb := ZoneBounds{Z1Hi: 116, Z2Hi: 145, Z3Hi: 157.5, Z4Hi: 170}
	s := Series{T: []float64{0, 1, 2, 3}, HR: []float64{120, 120, 130, 130}, V: []float64{2, 2, 2, 2}, Dist: []float64{0, 2, 4, 6}}
	a := Analyze(14820001234, s, zb)
	if a.ActivityID != 14820001234 {
		t.Errorf("ActivityID = %d", a.ActivityID)
	}
	if !a.HasHR {
		t.Error("HasHR = false, want true")
	}
	if len(a.TimeInZone) != 5 {
		t.Errorf("TimeInZone len = %d, want 5", len(a.TimeInZone))
	}
	if a.Zones != zb {
		t.Errorf("Zones = %+v, want snapshot %+v", a.Zones, zb)
	}
	if a.DecouplingPct == nil {
		t.Error("DecouplingPct nil, want a value")
	}
	if a.ComputedAt != "" {
		t.Errorf("ComputedAt = %q, want empty (caller sets)", a.ComputedAt)
	}
}

func TestAnalyzeNoHR(t *testing.T) {
	zb := ZoneBounds{Z1Hi: 116, Z2Hi: 145, Z3Hi: 157.5, Z4Hi: 170}
	s := Series{T: []float64{0, 1, 2}, HR: nil, V: []float64{2, 2, 2}, Dist: []float64{0, 2, 4}}
	a := Analyze(1, s, zb)
	if a.HasHR {
		t.Error("HasHR = true, want false")
	}
	if len(a.TimeInZone) != 0 {
		t.Errorf("TimeInZone = %v, want [] (no HR)", a.TimeInZone)
	}
	if a.DecouplingPct != nil || a.PaHRFirst != nil || a.PaHRSecond != nil {
		t.Errorf("decoupling fields not nil on no-HR: %+v", a)
	}
}

func TestStreamAnalysisJSONTags(t *testing.T) {
	pct := 4.2
	a := StreamAnalysis{
		ActivityID: 1, HasHR: true,
		TimeInZone:    []ZoneTime{{Zone: 1, Seconds: 120, Pct: 4}},
		DecouplingPct: &pct, Zones: ZoneBounds{Z1Hi: 116, Z2Hi: 145, Z3Hi: 157.5, Z4Hi: 170},
		Source: "strava", ComputedAt: "2026-06-22T07:00:00Z",
	}
	b, _ := json.Marshal(a)
	got := string(b)
	for _, k := range []string{
		`"activity_id":1`, `"has_hr":true`, `"decoupling_pct":4.2`,
		`"time_in_zone":[{"zone":1,"seconds":120,"pct":4}]`,
		`"zones":{"z1_hi":116,"z2_hi":145,"z3_hi":157.5,"z4_hi":170}`,
		`"source":"strava"`, `"computed_at":"2026-06-22T07:00:00Z"`,
	} {
		if !strings.Contains(got, k) {
			t.Errorf("JSON %s missing %q", got, k)
		}
	}
}

func TestZoneTimeJSONTags(t *testing.T) {
	b, _ := json.Marshal(ZoneTime{Zone: 2, Seconds: 2400, Pct: 80})
	if got := string(b); got != `{"zone":2,"seconds":2400,"pct":80}` {
		t.Errorf("ZoneTime JSON = %s", got)
	}
}
