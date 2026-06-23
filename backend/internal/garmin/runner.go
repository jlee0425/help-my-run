package garmin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
)

// Runner invokes the Python Garmin worker. Python and Script are injectable so
// tests can substitute a stub command (e.g. /bin/cat of a fixture).
type Runner struct {
	Python string
	Script string
}

// RunGarminFetch runs `<python> <script> fetch --since <since>` with extraEnv
// appended to the current environment, parses the worker's stdout JSON, and
// surfaces captured stderr in the error on non-zero exit.
func (r Runner) RunGarminFetch(ctx context.Context, since string, extraEnv []string) (*WorkerOutput, error) {
	cmd := exec.CommandContext(ctx, r.Python, r.Script, "fetch", "--since", since)
	if len(extraEnv) > 0 {
		cmd.Env = append(cmd.Environ(), extraEnv...)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return nil, fmt.Errorf("worker exit %d: %s", ee.ExitCode(), stderr.String())
		}
		return nil, fmt.Errorf("worker start failed: %w (stderr: %s)", err, stderr.String())
	}

	var out WorkerOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		return nil, fmt.Errorf("worker JSON parse: %w (stdout: %.200s)", err, stdout.String())
	}
	return &out, nil
}

// RunGarminFetchFIT runs `<python> <script> stream --activity-id <garminID>
// --echo-id <echoID>`, parsing the §2.6 stdout JSON. garminActivityID is the
// Garmin download id; echoActivityID is the activity id (identity: equals
// garminActivityID) echoed back as out.ActivityID so the store row keys
// correctly.
func (r Runner) RunGarminFetchFIT(ctx context.Context, garminActivityID, echoActivityID int64, extraEnv []string) (*FITStreamOutput, error) {
	cmd := exec.CommandContext(ctx, r.Python, r.Script, "stream",
		"--activity-id", strconv.FormatInt(garminActivityID, 10),
		"--echo-id", strconv.FormatInt(echoActivityID, 10))
	if len(extraEnv) > 0 {
		cmd.Env = append(cmd.Environ(), extraEnv...)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return nil, fmt.Errorf("worker exit %d: %s", ee.ExitCode(), stderr.String())
		}
		return nil, fmt.Errorf("worker start failed: %w (stderr: %s)", err, stderr.String())
	}

	var out FITStreamOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		return nil, fmt.Errorf("worker JSON parse: %w (stdout: %.200s)", err, stdout.String())
	}
	return &out, nil
}
