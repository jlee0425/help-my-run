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
			{"garmin_activity_id":14820001234,"name":"Morning Run",
			 "start_time":"2026-06-22T05:00:00Z","start_time_local":"2026-06-22 07:00:00",
			 "activity_type":"running","distance_m":10000.0,
			 "moving_time_s":3200.0,"elapsed_time_s":3300.0,
			 "avg_hr":148.0,"max_hr":168.0,"avg_speed":3.05,"max_speed":4.2,
			 "avg_cadence":172.0,"elevation_gain_m":85.0,
			 "raw_json":{"activityId":14820001234}},
			{"garmin_activity_id":14820005678,"name":null,
			 "start_time":"2026-06-21T06:00:00Z","start_time_local":null,
			 "activity_type":null,"distance_m":null,
			 "moving_time_s":null,"elapsed_time_s":null,
			 "avg_hr":null,"max_hr":null,"avg_speed":null,"max_speed":null,
			 "avg_cadence":null,"elevation_gain_m":null,
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
	if a.Name != "Morning Run" {
		t.Errorf("Name = %q, want Morning Run", a.Name)
	}
	if a.StartTime != "2026-06-22T05:00:00Z" {
		t.Errorf("StartTime = %q, want 2026-06-22T05:00:00Z", a.StartTime)
	}
	if a.StartTimeLocal == nil || *a.StartTimeLocal != "2026-06-22 07:00:00" {
		t.Errorf("StartTimeLocal = %v, want 2026-06-22 07:00:00", a.StartTimeLocal)
	}
	if a.DistanceM == nil || *a.DistanceM != 10000 {
		t.Errorf("DistanceM = %v, want 10000", a.DistanceM)
	}
	if a.ActivityType == nil || *a.ActivityType != "running" {
		t.Errorf("ActivityType = %v, want running", a.ActivityType)
	}
	if a.MovingTimeS == nil || *a.MovingTimeS != 3200 {
		t.Errorf("MovingTimeS = %v, want 3200", a.MovingTimeS)
	}
	if a.ElapsedTimeS == nil || *a.ElapsedTimeS != 3300 {
		t.Errorf("ElapsedTimeS = %v, want 3300", a.ElapsedTimeS)
	}
	if a.AvgHR == nil || *a.AvgHR != 148 {
		t.Errorf("AvgHR = %v, want 148", a.AvgHR)
	}
	if a.AvgCadence == nil || *a.AvgCadence != 172 {
		t.Errorf("AvgCadence = %v, want 172", a.AvgCadence)
	}
	if a.ElevationGainM == nil || *a.ElevationGainM != 85 {
		t.Errorf("ElevationGainM = %v, want 85", a.ElevationGainM)
	}
	if string(a.RawJSON) != `{"activityId":14820001234}` {
		t.Errorf("RawJSON = %s, want raw element", a.RawJSON)
	}
	// Null nested fields stay nil; name:null -> "" (Go string); raw_json:null -> "null".
	b := out.Activities[1]
	if b.Name != "" {
		t.Errorf("null name = %q, want empty string", b.Name)
	}
	if b.StartTimeLocal != nil || b.ActivityType != nil || b.DistanceM != nil ||
		b.MovingTimeS != nil || b.ElapsedTimeS != nil || b.AvgHR != nil ||
		b.AvgCadence != nil || b.ElevationGainM != nil {
		t.Errorf("null row: not all nullable fields are nil: %+v", b)
	}
	if string(b.RawJSON) != "null" {
		t.Errorf("null raw_json = %s, want literal null", b.RawJSON)
	}
}
