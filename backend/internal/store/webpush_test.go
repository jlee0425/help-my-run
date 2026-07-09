package store

import "testing"

func TestPushSubscriptionsCRUD(t *testing.T) {
	s := newTestStore(t)

	list, err := s.ListPushSubscriptions()
	if err != nil || len(list) != 0 {
		t.Fatalf("empty list = %v,%v", list, err)
	}
	if err := s.UpsertPushSubscription(PushSubscription{
		Endpoint: "https://push.example/a", P256dh: "k1", Auth: "a1",
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	// Same endpoint updates keys (browser re-subscribed).
	if err := s.UpsertPushSubscription(PushSubscription{
		Endpoint: "https://push.example/a", P256dh: "k2", Auth: "a2",
	}); err != nil {
		t.Fatalf("Upsert 2: %v", err)
	}
	if err := s.UpsertPushSubscription(PushSubscription{
		Endpoint: "https://push.example/b", P256dh: "k3", Auth: "a3",
	}); err != nil {
		t.Fatalf("Upsert 3: %v", err)
	}
	list, err = s.ListPushSubscriptions()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len = %d, want 2 (upsert must not duplicate)", len(list))
	}
	byEndpoint := map[string]PushSubscription{}
	for _, p := range list {
		if p.CreatedAt == "" {
			t.Errorf("created_at not stamped: %+v", p)
		}
		byEndpoint[p.Endpoint] = p
	}
	if byEndpoint["https://push.example/a"].P256dh != "k2" || byEndpoint["https://push.example/a"].Auth != "a2" {
		t.Fatalf("upsert did not update keys: %+v", byEndpoint["https://push.example/a"])
	}

	if err := s.DeletePushSubscription("https://push.example/a"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := s.DeletePushSubscription("https://push.example/missing"); err != nil {
		t.Fatalf("Delete missing should be a no-op: %v", err)
	}
	list, _ = s.ListPushSubscriptions()
	if len(list) != 1 || list[0].Endpoint != "https://push.example/b" {
		t.Fatalf("after delete = %+v", list)
	}
}

func TestProfileV2FieldsRoundTrip(t *testing.T) {
	s := newTestStore(t)

	p := AthleteProfile{
		TargetWeeklyKm:     30,
		ProgressionMode:    "build",
		RunConstraintsJSON: "{}",
		GoalText:           "engine",
		DailyRunTime:       "06:00",
		Timezone:           "UTC",
		AgentEnabled:       true,
		GoalsJSON:          `["crossfit","fitness"]`,
		WeekJSON:           `{"runs_per_week":4,"crossfit_days":3,"rest_day":"monday"}`,
		GuardrailsJSON:     `{"no_b2b_hard":true}`,
	}
	if err := s.UpsertAthleteProfile(p); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	got, err := s.GetAthleteProfile()
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.GoalsJSON != p.GoalsJSON || got.WeekJSON != p.WeekJSON || got.GuardrailsJSON != p.GuardrailsJSON {
		t.Fatalf("v2 fields = %q %q %q", got.GoalsJSON, got.WeekJSON, got.GuardrailsJSON)
	}
}
