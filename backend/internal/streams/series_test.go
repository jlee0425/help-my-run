package streams

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"reflect"
	"testing"
)

func TestSeriesHasHRAndLen(t *testing.T) {
	withHR := Series{T: []float64{0, 1, 2}, HR: []float64{100, 101, 102}, V: []float64{0, 1.5, 1.6}, Dist: []float64{0, 1.5, 3.1}}
	if !withHR.HasHR() {
		t.Error("HasHR() = false, want true when HR populated")
	}
	if withHR.Len() != 3 {
		t.Errorf("Len() = %d, want 3", withHR.Len())
	}
	noHR := Series{T: []float64{0, 1}, HR: nil, V: []float64{0, 1.5}, Dist: []float64{0, 1.5}}
	if noHR.HasHR() {
		t.Error("HasHR() = true, want false when HR empty")
	}
	if noHR.Len() != 2 {
		t.Errorf("Len() = %d, want 2", noHR.Len())
	}
}

func TestCompressDecompressRoundTrip(t *testing.T) {
	in := Series{
		T:    []float64{0, 1, 2, 3},
		HR:   []float64{104, 105, 106, 107},
		V:    []float64{0.0, 1.59, 1.66, 1.69},
		Dist: []float64{0.0, 2.9, 5.6, 8.4},
	}
	gz, err := CompressSeries(in)
	if err != nil {
		t.Fatalf("CompressSeries error = %v", err)
	}
	if len(gz) == 0 {
		t.Fatal("CompressSeries returned empty bytes")
	}
	// Bytes are real gzip (magic 0x1f 0x8b).
	if len(gz) < 2 || gz[0] != 0x1f || gz[1] != 0x8b {
		t.Errorf("CompressSeries output not gzip-framed: first bytes %v", gz[:min(2, len(gz))])
	}
	out, err := DecompressSeries(gz)
	if err != nil {
		t.Fatalf("DecompressSeries error = %v", err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Errorf("round-trip mismatch:\n in  = %+v\n out = %+v", in, out)
	}
}

func TestCompressDecompressNoHR(t *testing.T) {
	in := Series{T: []float64{0, 1, 2}, HR: nil, V: []float64{0, 1.5, 1.6}, Dist: []float64{0, 1.5, 3.1}}
	gz, err := CompressSeries(in)
	if err != nil {
		t.Fatalf("CompressSeries error = %v", err)
	}
	out, err := DecompressSeries(gz)
	if err != nil {
		t.Fatalf("DecompressSeries error = %v", err)
	}
	if out.HasHR() {
		t.Errorf("round-tripped no-HR series HasHR() = true, want false (HR=%v)", out.HR)
	}
	if out.Len() != 3 {
		t.Errorf("Len() = %d, want 3", out.Len())
	}
}

func TestDecompressSeriesRejectsGarbage(t *testing.T) {
	if _, err := DecompressSeries([]byte("not gzip")); err == nil {
		t.Error("DecompressSeries(garbage) error = nil, want non-nil")
	}
}

// verifies the gzipped payload is the canonical struct-of-arrays JSON.
func TestCompressSeriesPayloadIsSnakeArrays(t *testing.T) {
	in := Series{T: []float64{0}, HR: []float64{100}, V: []float64{1.5}, Dist: []float64{0}}
	gz, err := CompressSeries(in)
	if err != nil {
		t.Fatalf("CompressSeries error = %v", err)
	}
	r, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		t.Fatalf("gzip.NewReader error = %v", err)
	}
	raw, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read gunzipped: %v", err)
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		t.Fatalf("gunzipped payload not JSON object: %v", err)
	}
	for _, k := range []string{"t", "hr", "v", "dist"} {
		if _, ok := probe[k]; !ok {
			t.Errorf("payload missing key %q; got %v", k, raw)
		}
	}
}
