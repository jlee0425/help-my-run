package garmin

import (
	"encoding/json"
	"testing"
)

func TestWorkerOutputUnmarshalActivities(t *testing.T) {
	const blob = `{
		"since":"2026-06-14","until":"2026-06-15","fetched_at":"t",
		"sleep":[],"hrv":[],"body_battery":[],"rhr":[],"vo2max":[],
		"activities":[
			{"garmin_activity_id":14820001234,"start_time":"2026-06-22 05:00:00",
			 "duration_s":3300.0,"distance_m":10000.0,"activity_type":"running",
			 "raw_json":{"activityId":14820001234}},
			{"garmin_activity_id":14820005678,"start_time":"2026-06-21 06:00:00",
			 "duration_s":null,"distance_m":null,"activity_type":null,
			 "raw_json":null}
		]
	}`
	var out WorkerOutput
	if err := json.Unmarshal([]byte(blob), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Activities) != 2 {
		t.Fatalf("Activities len = %d, want 2", len(out.Activities))
	}
	a := out.Activities[0]
	if a.GarminActivityID != 14820001234 {
		t.Errorf("GarminActivityID = %d, want 14820001234", a.GarminActivityID)
	}
	if a.StartTime != "2026-06-22 05:00:00" {
		t.Errorf("StartTime = %q, want 2026-06-22 05:00:00", a.StartTime)
	}
	if a.DurationS == nil || *a.DurationS != 3300 {
		t.Errorf("DurationS = %v, want 3300", a.DurationS)
	}
	if a.DistanceM == nil || *a.DistanceM != 10000 {
		t.Errorf("DistanceM = %v, want 10000", a.DistanceM)
	}
	if a.ActivityType == nil || *a.ActivityType != "running" {
		t.Errorf("ActivityType = %v, want running", a.ActivityType)
	}
	if string(a.RawJSON) != `{"activityId":14820001234}` {
		t.Errorf("RawJSON = %s, want raw element", a.RawJSON)
	}
	// Null nested fields stay nil; raw_json:null -> RawJSON == "null".
	b := out.Activities[1]
	if b.DurationS != nil || b.DistanceM != nil || b.ActivityType != nil {
		t.Errorf("null row: dur=%v dist=%v atype=%v, want all nil", b.DurationS, b.DistanceM, b.ActivityType)
	}
	if string(b.RawJSON) != "null" {
		t.Errorf("null raw_json = %s, want literal null", b.RawJSON)
	}
}
