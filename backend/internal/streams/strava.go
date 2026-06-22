package streams

import "help-my-run/backend/internal/strava"

// FromStravaStreams maps a Strava StreamSet to the normalized Series.
// t<-time, v<-velocity_smooth, dist<-distance, hr<-heartrate (or [] if absent).
// All arrays are truncated to the shortest PRESENT length to stay index-aligned.
func FromStravaStreams(ss strava.StreamSet) Series {
	t := ss["time"].Data
	v := ss["velocity_smooth"].Data
	dist := ss["distance"].Data
	hr, hasHR := ss["heartrate"]

	n := minLen(len(t), len(v), len(dist))
	if hasHR {
		n = minLen(n, len(hr.Data))
	}
	out := Series{
		T:    clip(t, n),
		V:    clip(v, n),
		Dist: clip(dist, n),
	}
	if hasHR {
		out.HR = clip(hr.Data, n)
	}
	return out
}

func minLen(xs ...int) int {
	m := xs[0]
	for _, x := range xs[1:] {
		if x < m {
			m = x
		}
	}
	return m
}

func clip(a []float64, n int) []float64 {
	if n > len(a) {
		n = len(a)
	}
	return append([]float64(nil), a[:n]...)
}
