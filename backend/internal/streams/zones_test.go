package streams

import (
	"encoding/json"
	"strings"
	"testing"

	"help-my-run/backend/internal/store"
)

func i64(v int64) *int64 { return &v }

func TestZonesFromProfileExplicit(t *testing.T) {
	p := store.AthleteProfile{Zone2CeilingBpm: i64(150), ThresholdBpm: i64(168), MaxHRBpm: i64(186)}
	zb := ZonesFromProfile(p)
	if zb.Z2Hi != 150 {
		t.Errorf("Z2Hi = %v, want 150 (zone2 ceiling)", zb.Z2Hi)
	}
	if zb.Z4Hi != 168 {
		t.Errorf("Z4Hi = %v, want 168 (threshold)", zb.Z4Hi)
	}
	// z1Hi = 0.80 * z2Hi = 120; z3Hi = (150+168)/2 = 159.
	if zb.Z1Hi != 0.80*150 {
		t.Errorf("Z1Hi = %v, want %v (0.80*z2Hi)", zb.Z1Hi, 0.80*150)
	}
	if zb.Z3Hi != (150+168)/2.0 {
		t.Errorf("Z3Hi = %v, want %v (midpoint)", zb.Z3Hi, (150+168)/2.0)
	}
}

func TestZonesFromProfileDefaults(t *testing.T) {
	zb := ZonesFromProfile(store.AthleteProfile{}) // all nil -> 145/170/190
	if zb.Z2Hi != DefaultZone2Hi || zb.Z4Hi != DefaultThreshold {
		t.Errorf("defaults z2Hi/z4Hi = %v/%v, want %v/%v", zb.Z2Hi, zb.Z4Hi, DefaultZone2Hi, DefaultThreshold)
	}
	if zb.Z1Hi != 0.80*DefaultZone2Hi {
		t.Errorf("Z1Hi = %v, want %v", zb.Z1Hi, 0.80*DefaultZone2Hi)
	}
	if zb.Z3Hi != (DefaultZone2Hi+DefaultThreshold)/2.0 {
		t.Errorf("Z3Hi = %v, want %v", zb.Z3Hi, (DefaultZone2Hi+DefaultThreshold)/2.0)
	}
}

func TestZoneDefaultConstants(t *testing.T) {
	if DefaultZone2Hi != 145.0 || DefaultThreshold != 170.0 || DefaultMaxHRBpm != 190.0 {
		t.Errorf("zone defaults drifted: %v/%v/%v", DefaultZone2Hi, DefaultThreshold, DefaultMaxHRBpm)
	}
}

func TestZoneBoundsJSONTags(t *testing.T) {
	b, _ := json.Marshal(ZoneBounds{Z1Hi: 116, Z2Hi: 145, Z3Hi: 157.5, Z4Hi: 170})
	got := string(b)
	for _, k := range []string{`"z1_hi":116`, `"z2_hi":145`, `"z3_hi":157.5`, `"z4_hi":170`} {
		if !strings.Contains(got, k) {
			t.Errorf("JSON %s missing %q", got, k)
		}
	}
}
