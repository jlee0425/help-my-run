package streams

import "help-my-run/backend/internal/store"

// Documented fallbacks (used only when the corresponding profile field is nil).
// DefaultZone2Hi matches progress.defaultRefHRBpm (145).
const (
	DefaultMaxHRBpm  = 190.0 // when MaxHRBpm nil
	DefaultZone2Hi   = 145.0 // when Zone2CeilingBpm nil
	DefaultThreshold = 170.0 // when ThresholdBpm nil
)

// ZoneBounds are the 4 internal boundaries (z1Hi..z4Hi) used for a computation,
// snapshotted into stream_analyses.zones_json so a profile change triggers
// recompute. 5 zones: Z1 < z1Hi, Z2 [z1Hi,z2Hi), Z3 [z2Hi,z3Hi), Z4 [z3Hi,z4Hi),
// Z5 >= z4Hi.
type ZoneBounds struct {
	Z1Hi float64 `json:"z1_hi"` // Z1->Z2 boundary (recovery ceiling)
	Z2Hi float64 `json:"z2_hi"` // Z2->Z3 (zone2 ceiling)
	Z3Hi float64 `json:"z3_hi"` // Z3->Z4 (tempo midpoint)
	Z4Hi float64 `json:"z4_hi"` // Z4->Z5 (threshold)
}

// ZonesFromProfile derives the 5-zone boundaries from the profile + defaults.
// z2Hi = Zone2CeilingBpm (or DefaultZone2Hi); z4Hi = ThresholdBpm (or
// DefaultThreshold); z1Hi = 0.80*z2Hi (recovery ceiling); z3Hi = midpoint(z2Hi,
// z4Hi). MaxHRBpm bounds Z5's notional top but Z5 is open-ended for bucketing.
func ZonesFromProfile(p store.AthleteProfile) ZoneBounds {
	z2Hi := DefaultZone2Hi
	if p.Zone2CeilingBpm != nil {
		z2Hi = float64(*p.Zone2CeilingBpm)
	}
	z4Hi := DefaultThreshold
	if p.ThresholdBpm != nil {
		z4Hi = float64(*p.ThresholdBpm)
	}
	return ZoneBounds{
		Z1Hi: 0.80 * z2Hi,
		Z2Hi: z2Hi,
		Z3Hi: (z2Hi + z4Hi) / 2.0,
		Z4Hi: z4Hi,
	}
}
