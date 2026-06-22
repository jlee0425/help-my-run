package streams

// ZoneTime is one HR zone's dwell time + share of moving time. JSON is the
// stored time_in_zone_json element AND the wire DTO element (snake_case).
type ZoneTime struct {
	Zone    int     `json:"zone"`    // 1..5
	Seconds float64 `json:"seconds"` // dwell time in this zone
	Pct     float64 `json:"pct"`     // 0..100 share of total HR-sampled seconds
}

// zoneOf buckets a single HR value into a 1..5 zone using inclusive-low,
// exclusive-high boundaries (top zone open-ended).
func zoneOf(hr float64, zb ZoneBounds) int {
	switch {
	case hr < zb.Z1Hi:
		return 1
	case hr < zb.Z2Hi:
		return 2
	case hr < zb.Z3Hi:
		return 3
	case hr < zb.Z4Hi:
		return 4
	default:
		return 5
	}
}

// sampleDT returns the dwell time attributed to sample i: t[i+1]-t[i], with the
// last sample reusing the previous dt (1.0 fallback for a single sample).
func sampleDT(t []float64, i int) float64 {
	n := len(t)
	if n < 2 {
		return 1.0
	}
	if i < n-1 {
		return t[i+1] - t[i]
	}
	return t[n-1] - t[n-2] // last sample reuses prior dt
}

// TimeInZone buckets each sample's HR into a zone, accumulating dt between
// consecutive t[] samples. Returns exactly 5 ZoneTime entries (Z1..Z5); pct is
// relative to summed dt. Empty / no-HR series -> empty slice (len 0).
func TimeInZone(s Series, zb ZoneBounds) []ZoneTime {
	if !s.HasHR() {
		return []ZoneTime{}
	}
	n := s.Len()
	if len(s.HR) < n {
		n = len(s.HR)
	}
	secs := make([]float64, 5) // index 0..4 -> Z1..Z5
	var total float64
	for i := 0; i < n; i++ {
		dt := sampleDT(s.T, i)
		z := zoneOf(s.HR[i], zb)
		secs[z-1] += dt
		total += dt
	}
	out := make([]ZoneTime, 5)
	for z := 0; z < 5; z++ {
		pct := 0.0
		if total > 0 {
			pct = secs[z] / total * 100
		}
		out[z] = ZoneTime{Zone: z + 1, Seconds: secs[z], Pct: pct}
	}
	return out
}

// Decoupling is the aerobic-durability drift result.
type Decoupling struct {
	DecouplingPct *float64 `json:"decoupling_pct"` // nil when not computable
	PaHRFirst     *float64 `json:"pa_hr_first"`    // first-half speed-per-beat (m/beat), nil if N/A
	PaHRSecond    *float64 `json:"pa_hr_second"`   // second-half, nil if N/A
}

// meanPaHR returns mean(v)/mean(hr) over [lo,hi) and whether it is computable
// (>=2 samples and mean(hr) != 0).
func meanPaHR(s Series, lo, hi int) (float64, bool) {
	count := hi - lo
	if count < 2 {
		return 0, false
	}
	var sumV, sumHR float64
	for i := lo; i < hi; i++ {
		sumV += s.V[i]
		sumHR += s.HR[i]
	}
	meanHR := sumHR / float64(count)
	if meanHR == 0 {
		return 0, false
	}
	meanV := sumV / float64(count)
	return meanV / meanHR, true
}

// ComputeDecoupling splits the series at the MOVING-TIME MIDPOINT (half of total
// elapsed t span), computes Pa:HR = mean(v)/mean(hr) for each half, and the
// drift: decoupling_pct = (paHRFirst - paHRSecond) / paHRFirst * 100. Higher
// drift = worse durability (lower_is_better). Returns all-nil when: no HR, < 2
// samples per half, mean(hr)==0 in a half, or paHRFirst==0.
func ComputeDecoupling(s Series) Decoupling {
	if !s.HasHR() {
		return Decoupling{}
	}
	n := s.Len()
	if len(s.HR) < n {
		n = len(s.HR)
	}
	if len(s.V) < n {
		n = len(s.V)
	}
	if n < 4 { // need >=2 samples per half
		return Decoupling{}
	}
	tMid := (s.T[0] + s.T[n-1]) / 2.0
	split := 0
	for split < n && s.T[split] <= tMid {
		split++
	}
	first, ok1 := meanPaHR(s, 0, split)
	second, ok2 := meanPaHR(s, split, n)
	if !ok1 || !ok2 || first == 0 {
		return Decoupling{}
	}
	pct := (first - second) / first * 100
	return Decoupling{DecouplingPct: &pct, PaHRFirst: &first, PaHRSecond: &second}
}

// StreamAnalysis is the cached per-run analysis (one stream_analyses row + the
// wire DTO source).
type StreamAnalysis struct {
	ActivityID    int64      `json:"activity_id"`
	HasHR         bool       `json:"has_hr"`
	TimeInZone    []ZoneTime `json:"time_in_zone"` // [] when !HasHR
	DecouplingPct *float64   `json:"decoupling_pct"`
	PaHRFirst     *float64   `json:"pa_hr_first"`
	PaHRSecond    *float64   `json:"pa_hr_second"`
	Zones         ZoneBounds `json:"zones"`       // boundaries used (snapshot)
	Source        string     `json:"source"`      // "strava"|"garmin"; engine sets
	ComputedAt    string     `json:"computed_at"` // RFC3339 UTC; caller sets
}

// Analyze runs TimeInZone + ComputeDecoupling over a decompressed Series with the
// given zone boundaries. Pure; the caller sets ActivityID + Source + ComputedAt
// (Source/ComputedAt left empty here).
func Analyze(activityID int64, s Series, zb ZoneBounds) StreamAnalysis {
	dec := ComputeDecoupling(s)
	return StreamAnalysis{
		ActivityID:    activityID,
		HasHR:         s.HasHR(),
		TimeInZone:    TimeInZone(s, zb),
		DecouplingPct: dec.DecouplingPct,
		PaHRFirst:     dec.PaHRFirst,
		PaHRSecond:    dec.PaHRSecond,
		Zones:         zb,
	}
}
