package readiness

import (
	"math"
	"testing"
)

func TestRoundHalf(t *testing.T) {
	cases := []struct {
		in   float64
		want float64
	}{
		{4.0, 4.0}, {4.2, 4.0}, {4.25, 4.5}, {4.74, 4.5}, {4.75, 5.0},
		{6.0, 6.0}, {3.1, 3.0}, {0.0, 0.0}, {2.5, 2.5},
	}
	for _, c := range cases {
		if got := roundHalf(c.in); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("roundHalf(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestIsEasyType(t *testing.T) {
	cases := map[string]bool{
		"easy": true, "recovery": true, "rest": true,
		"tempo": false, "intervals": false, "long": false, "": false,
	}
	for typ, want := range cases {
		if got := isEasyType(typ); got != want {
			t.Errorf("isEasyType(%q) = %v, want %v", typ, got, want)
		}
	}
}

func TestFallback(t *testing.T) {
	const easyPace = "6:00/km"

	tempo := &FallbackSession{
		Date: "2026-06-20", Dow: "Fri", RunType: "tempo", DistanceKm: 6,
		PaceTarget: "5:05/km", TimeNote: "~20:00 after CrossFit",
	}
	easy := &FallbackSession{
		Date: "2026-06-20", Dow: "Fri", RunType: "easy", DistanceKm: 8,
		PaceTarget: "6:00/km", TimeNote: "~20:00 after CrossFit",
	}

	tests := []struct {
		name       string
		color      Color
		session    *FallbackSession
		wantAction string
		wantAdjNil bool
		wantType   string
		wantDistKm float64
		wantPace   string
		wantOptCNS bool
	}{
		{
			name:  "RED + quality run -> MOVE to easy recovery, capped 4km",
			color: ColorRed, session: tempo,
			wantAction: "MOVE", wantType: "recovery", wantDistKm: 4, wantPace: easyPace, wantOptCNS: true,
		},
		{
			name:  "RED + already easy -> SOFTEN to half, easy pace",
			color: ColorRed, session: easy,
			wantAction: "SOFTEN", wantType: "easy", wantDistKm: 4, wantPace: easyPace, wantOptCNS: true,
		},
		{
			name:  "RED + no run -> REST_DAY",
			color: ColorRed, session: nil,
			wantAction: "REST_DAY", wantAdjNil: true,
		},
		{
			name:  "AMBER + quality -> SOFTEN to 75%, easy pace",
			color: ColorAmber, session: tempo,
			wantAction: "SOFTEN", wantType: "tempo", wantDistKm: 4.5, wantPace: easyPace, wantOptCNS: false,
		},
		{
			name:  "AMBER + easy -> STAND unchanged",
			color: ColorAmber, session: easy,
			wantAction: "STAND", wantType: "easy", wantDistKm: 8, wantPace: "6:00/km", wantOptCNS: false,
		},
		{
			name:  "AMBER + no run -> REST_DAY",
			color: ColorAmber, session: nil,
			wantAction: "REST_DAY", wantAdjNil: true,
		},
		{
			name:  "GREEN + quality -> STAND unchanged",
			color: ColorGreen, session: tempo,
			wantAction: "STAND", wantType: "tempo", wantDistKm: 6, wantPace: "5:05/km", wantOptCNS: false,
		},
		{
			name:  "GREEN + no run -> REST_DAY",
			color: ColorGreen, session: nil,
			wantAction: "REST_DAY", wantAdjNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dec := Fallback(tt.color, tt.session, easyPace)
			if string(dec.Action) != tt.wantAction {
				t.Fatalf("Action = %q, want %q", dec.Action, tt.wantAction)
			}
			if dec.Rationale == "" {
				t.Errorf("Rationale empty, want non-empty")
			}
			if tt.wantAdjNil {
				if dec.Adjusted != nil {
					t.Errorf("Adjusted = %+v, want nil", dec.Adjusted)
				}
				return
			}
			if dec.Adjusted == nil {
				t.Fatalf("Adjusted = nil, want session")
			}
			a := dec.Adjusted
			if a.RunType != tt.wantType {
				t.Errorf("RunType = %q, want %q", a.RunType, tt.wantType)
			}
			if math.Abs(a.DistanceKm-tt.wantDistKm) > 1e-9 {
				t.Errorf("DistanceKm = %v, want %v", a.DistanceKm, tt.wantDistKm)
			}
			if a.PaceTarget != tt.wantPace {
				t.Errorf("PaceTarget = %q, want %q", a.PaceTarget, tt.wantPace)
			}
			if a.OptionalIfCNS != tt.wantOptCNS {
				t.Errorf("OptionalIfCNS = %v, want %v", a.OptionalIfCNS, tt.wantOptCNS)
			}
			if tt.wantAction == "STAND" {
				if a.Date != tt.session.Date || a.Dow != tt.session.Dow || a.TimeNote != tt.session.TimeNote {
					t.Errorf("STAND mutated identity fields: %+v", a)
				}
			}
		})
	}
}

func TestFallbackActionConstants(t *testing.T) {
	if FbStand != "STAND" || FbSoften != "SOFTEN" || FbMove != "MOVE" || FbRestDay != "REST_DAY" {
		t.Errorf("action constants drifted: %q %q %q %q", FbStand, FbSoften, FbMove, FbRestDay)
	}
}
