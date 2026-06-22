package garmin

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRunGarminFetchParsesOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh")
	}
	fixture, err := filepath.Abs(filepath.Join("testdata", "worker_output.json"))
	if err != nil {
		t.Fatalf("abs path: %v", err)
	}
	// Stub "worker": a shell script that cats the fixture and ignores the fetch
	// args, printing the JSON to stdout. (GNU coreutils `cat` would reject the
	// `--since` flag as an unknown option, so a script is used instead of /bin/cat.)
	script := filepath.Join(t.TempDir(), "stub.sh")
	body := "#!/bin/sh\ncat '" + fixture + "'\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	r := Runner{Python: "/bin/sh", Script: script}

	out, err := r.RunGarminFetch(context.Background(), "2026-06-14", nil)
	if err != nil {
		t.Fatalf("RunGarminFetch error = %v", err)
	}
	if out.Since != "2026-06-14" || out.Until != "2026-06-15" {
		t.Errorf("since/until = %q/%q, want 2026-06-14/2026-06-15", out.Since, out.Until)
	}
	if len(out.Sleep) != 2 || out.Sleep[0].Date != "2026-06-14" || *out.Sleep[0].DurationS != 26400 {
		t.Errorf("sleep parse wrong: %+v", out.Sleep)
	}
	if len(out.HRV) != 1 || out.HRV[0].Status == nil || *out.HRV[0].Status != "BALANCED" {
		t.Errorf("hrv parse wrong: %+v", out.HRV)
	}
	if len(out.BodyBattery) != 2 || *out.BodyBattery[1].High != 91 {
		t.Errorf("body_battery parse wrong: %+v", out.BodyBattery)
	}
	if len(out.RHR) != 2 || *out.RHR[1].RestingHR != 47 {
		t.Errorf("rhr parse wrong: %+v", out.RHR)
	}
	if len(out.VO2Max) != 2 || out.VO2Max[1].VO2Max == nil || *out.VO2Max[1].VO2Max != 52.0 {
		t.Errorf("vo2max parse wrong: %+v", out.VO2Max)
	}
	// raw_json must be preserved as a JSON string for the store.
	if !strings.Contains(string(out.Sleep[0].RawJSON), "dailySleepDTO") {
		t.Errorf("sleep raw_json missing: %s", out.Sleep[0].RawJSON)
	}
}

func TestRunGarminFetchSurfacesStderrOnFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh")
	}
	// Stub script: print the login hint to stderr and exit 1.
	script := filepath.Join(t.TempDir(), "fail.sh")
	body := "#!/bin/sh\necho 're-run worker.py login' 1>&2\nexit 1\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	r := Runner{Python: "/bin/sh", Script: script}

	_, err := r.RunGarminFetch(context.Background(), "2026-06-14", nil)
	if err == nil {
		t.Fatal("RunGarminFetch error = nil, want non-nil on exit 1")
	}
	if !strings.Contains(err.Error(), "re-run worker.py login") {
		t.Errorf("error = %q, want it to contain stderr 're-run worker.py login'", err.Error())
	}
}

func TestRunGarminFetchFITParsesOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh")
	}
	const want = `{"activity_id":14820001234,"source":"garmin","fetched_at":"2026-06-22T05:00:12Z","series":{"t":[0,1,2],"hr":[104,105,106],"v":[0.0,1.59,1.66],"dist":[0.0,2.9,5.6]}}`
	script := filepath.Join(t.TempDir(), "stub.sh")
	body := "#!/bin/sh\n" +
		"echo \"$@\" | grep -q -- '--activity-id 555' || { echo 'missing --activity-id' 1>&2; exit 2; }\n" +
		"echo \"$@\" | grep -q -- '--echo-id 14820001234' || { echo 'missing --echo-id' 1>&2; exit 2; }\n" +
		"echo '" + want + "'\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	r := Runner{Python: "/bin/sh", Script: script}

	out, err := r.RunGarminFetchFIT(context.Background(), 555, 14820001234, nil)
	if err != nil {
		t.Fatalf("RunGarminFetchFIT error = %v", err)
	}
	if out.ActivityID != 14820001234 || out.Source != "garmin" {
		t.Errorf("out = id %d src %q, want 14820001234 garmin", out.ActivityID, out.Source)
	}
	if len(out.Series.HR) != 3 || out.Series.HR[0] != 104 {
		t.Errorf("series.hr = %v, want [104 105 106]", out.Series.HR)
	}
	if len(out.Series.V) != 3 || out.Series.V[1] != 1.59 {
		t.Errorf("series.v = %v, want [0 1.59 1.66]", out.Series.V)
	}
}
