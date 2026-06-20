package llm

import (
	"context"
	"errors"
	"testing"
)

// seqRunner returns a sequence of (out,err) per call; records call count.
type seqRunner struct {
	outs  [][]byte
	errs  []error
	calls int
}

func (r *seqRunner) Run(ctx context.Context, args []string, stdin string) ([]byte, error) {
	i := r.calls
	r.calls++
	var err error
	if i < len(r.errs) {
		err = r.errs[i]
	}
	return r.outs[i], err
}

func TestCallSuccess(t *testing.T) {
	r := &seqRunner{outs: [][]byte{loadFixture(t, "stage1_envelope.json")}, errs: []error{nil}}
	c := Client{Runner: r, Model: "claude-opus-4-8"}

	var w struct {
		WeekStart string `json:"week_start"`
	}
	if err := c.Call(context.Background(), []string{"-p", "x"}, "", &w); err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if w.WeekStart != "2026-06-22" {
		t.Errorf("week_start = %q, want 2026-06-22", w.WeekStart)
	}
	if r.calls != 1 {
		t.Errorf("calls = %d, want 1 (no retry on success)", r.calls)
	}
}

func TestCallRetriesOnceOnMalformed(t *testing.T) {
	r := &seqRunner{
		outs: [][]byte{loadFixture(t, "malformed_envelope.json"), loadFixture(t, "stage1_envelope.json")},
		errs: []error{nil, nil},
	}
	c := Client{Runner: r, Model: "claude-opus-4-8"}

	var w struct {
		WeekStart string `json:"week_start"`
	}
	if err := c.Call(context.Background(), []string{"-p", "x"}, "", &w); err != nil {
		t.Fatalf("Call() error = %v, want success on retry", err)
	}
	if r.calls != 2 {
		t.Errorf("calls = %d, want 2 (one retry)", r.calls)
	}
	if w.WeekStart != "2026-06-22" {
		t.Errorf("week_start = %q after retry, want 2026-06-22", w.WeekStart)
	}
}

func TestCallMalformedTwiceReturnsErrMalformed(t *testing.T) {
	r := &seqRunner{
		outs: [][]byte{loadFixture(t, "malformed_envelope.json"), loadFixture(t, "malformed_envelope.json")},
		errs: []error{nil, nil},
	}
	c := Client{Runner: r, Model: "claude-opus-4-8"}

	var v map[string]any
	err := c.Call(context.Background(), []string{"-p", "x"}, "", &v)
	if !errors.Is(err, ErrMalformedJSON) {
		t.Errorf("err = %v, want ErrMalformedJSON", err)
	}
	if r.calls != 2 {
		t.Errorf("calls = %d, want 2 (one retry then give up)", r.calls)
	}
}

func TestCallFailsOnIsErrorNoRetry(t *testing.T) {
	r := &seqRunner{outs: [][]byte{loadFixture(t, "not_logged_in_envelope.json")}, errs: []error{nil}}
	c := Client{Runner: r, Model: "claude-opus-4-8"}

	var v map[string]any
	err := c.Call(context.Background(), []string{"-p", "x"}, "", &v)
	if err == nil {
		t.Fatal("Call() error = nil, want failure on is_error")
	}
	if errors.Is(err, ErrMalformedJSON) {
		t.Errorf("err = %v, want a classified failure (not malformed)", err)
	}
	if r.calls != 1 {
		t.Errorf("calls = %d, want 1 (is_error is not retried)", r.calls)
	}
}
