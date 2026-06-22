package streams

import (
	"testing"

	"help-my-run/backend/internal/strava"
)

func TestFromStravaStreamsWithHR(t *testing.T) {
	ss := strava.StreamSet{
		"time":            {Data: []float64{0, 1, 2, 3}},
		"heartrate":       {Data: []float64{104, 105, 106, 107}},
		"velocity_smooth": {Data: []float64{0, 1.59, 1.66, 1.69}},
		"distance":        {Data: []float64{0, 2.9, 5.6, 8.4}},
	}
	s := FromStravaStreams(ss)
	if !s.HasHR() || s.Len() != 4 || s.HR[3] != 107 || s.V[1] != 1.59 || s.Dist[2] != 5.6 {
		t.Errorf("series = %+v, want 4 aligned samples", s)
	}
}

func TestFromStravaStreamsNoHR(t *testing.T) {
	ss := strava.StreamSet{
		"time":            {Data: []float64{0, 1}},
		"velocity_smooth": {Data: []float64{0, 1.59}},
		"distance":        {Data: []float64{0, 2.9}},
	}
	s := FromStravaStreams(ss)
	if s.HasHR() {
		t.Error("HasHR = true, want false when heartrate key absent")
	}
	if s.Len() != 2 {
		t.Errorf("Len = %d, want 2", s.Len())
	}
}

func TestFromStravaStreamsTruncatesToShortest(t *testing.T) {
	ss := strava.StreamSet{
		"time":            {Data: []float64{0, 1, 2}},
		"heartrate":       {Data: []float64{104, 105}}, // shortest present
		"velocity_smooth": {Data: []float64{0, 1.59, 1.66}},
		"distance":        {Data: []float64{0, 2.9, 5.6}},
	}
	s := FromStravaStreams(ss)
	if s.Len() != 2 || len(s.HR) != 2 {
		t.Errorf("len = %d / hr %d, want 2 / 2 (truncated to shortest)", s.Len(), len(s.HR))
	}
}
