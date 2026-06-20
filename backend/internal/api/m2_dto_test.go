package api

import (
	"encoding/json"
	"testing"
)

func TestValidISODate(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"2026-06-20", true},
		{"2026-13-01", false},
		{"2026-6-1", false},
		{"", false},
		{"../etc", false},
		{"2026-06-20T00:00:00Z", false},
	}
	for _, c := range cases {
		if got := validISODate(c.in); got != c.want {
			t.Errorf("validISODate(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestProfileDTOM2Fields(t *testing.T) {
	dto := profileDTO{DailyRunTime: "05:30", Timezone: "Asia/Seoul", AgentEnabled: true}
	b, _ := json.Marshal(dto)
	s := string(b)
	for _, want := range []string{`"daily_run_time":"05:30"`, `"timezone":"Asia/Seoul"`, `"agent_enabled":true`} {
		if !contains(s, want) {
			t.Errorf("profileDTO JSON %s missing %s", s, want)
		}
	}
}

func TestTodayResponseDTOTags(t *testing.T) {
	dto := todayResponseDTO{
		Date:           "2026-06-20",
		ReadinessColor: "amber",
		Drivers:        readinessDriversDTO{Date: "2026-06-20", DataComplete: true},
		Reasons:        []string{"HRV -18% vs baseline"},
		Action:         "SOFTEN",
		Rationale:      "trimmed",
		Source:         "ai",
		Stale:          false,
	}
	b, _ := json.Marshal(dto)
	s := string(b)
	for _, want := range []string{
		`"readiness_color":"amber"`, `"drivers":`, `"data_complete":true`,
		`"original_session":null`, `"effective_session":null`,
		`"reasons":["HRV -18% vs baseline"]`, `"action":"SOFTEN"`, `"source":"ai"`, `"stale":false`,
	} {
		if !contains(s, want) {
			t.Errorf("todayResponseDTO JSON %s missing %s", s, want)
		}
	}
}

func TestPushRegisterDTOTags(t *testing.T) {
	req := pushRegisterRequestDTO{ExpoPushToken: "ExponentPushToken[x]", Platform: "ios"}
	b, _ := json.Marshal(req)
	if !contains(string(b), `"expo_push_token":"ExponentPushToken[x]"`) || !contains(string(b), `"platform":"ios"`) {
		t.Errorf("pushRegisterRequestDTO JSON = %s", b)
	}
	resp := pushRegisterResponseDTO{ExpoPushToken: "ExponentPushToken[x]", Platform: "ios", UpdatedAt: "2026-06-20T05:30:01Z"}
	rb, _ := json.Marshal(resp)
	if !contains(string(rb), `"updated_at":"2026-06-20T05:30:01Z"`) {
		t.Errorf("pushRegisterResponseDTO JSON = %s", rb)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
