// Package streams owns the M3.2 per-sample stream model: the normalized
// struct-of-arrays Series, gzip (de)compression for storage, source
// normalization, the deterministic time-in-zone / decoupling engine, and the
// DB-loading Engine wrapper.
package streams

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
)

// Series is the normalized per-sample stream, struct-of-arrays, index-aligned.
// T is elapsed seconds since start; V is m/s; Dist is cumulative meters; HR is bpm.
// HR is empty (len 0) when the source stream carried no heart-rate sensor data.
type Series struct {
	T    []float64 `json:"t"`
	HR   []float64 `json:"hr"`
	V    []float64 `json:"v"`
	Dist []float64 `json:"dist"`
}

// HasHR reports whether the series carries heart-rate samples.
func (s Series) HasHR() bool { return len(s.HR) > 0 }

// Len is the number of time samples in the series.
func (s Series) Len() int { return len(s.T) }

// CompressSeries marshals s to JSON and gzips it (best compression). The result
// is stored verbatim in activity_streams.series_gz (a BLOB).
func CompressSeries(s Series) ([]byte, error) {
	raw, err := json.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("streams: marshal series: %w", err)
	}
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return nil, fmt.Errorf("streams: new gzip writer: %w", err)
	}
	if _, err := zw.Write(raw); err != nil {
		_ = zw.Close()
		return nil, fmt.Errorf("streams: gzip write: %w", err)
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("streams: gzip close: %w", err)
	}
	return buf.Bytes(), nil
}

// DecompressSeries gunzips gz and unmarshals it back to a Series.
func DecompressSeries(gz []byte) (Series, error) {
	zr, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		return Series{}, fmt.Errorf("streams: new gzip reader: %w", err)
	}
	defer func() { _ = zr.Close() }()
	raw, err := io.ReadAll(zr)
	if err != nil {
		return Series{}, fmt.Errorf("streams: gzip read: %w", err)
	}
	var s Series
	if err := json.Unmarshal(raw, &s); err != nil {
		return Series{}, fmt.Errorf("streams: unmarshal series: %w", err)
	}
	return s, nil
}
