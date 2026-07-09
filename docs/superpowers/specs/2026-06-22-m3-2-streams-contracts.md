# M3.2 Streams — Canonical Shared Contracts

**Date:** 2026-06-22
**Status:** Canonical (locked for implementation)
**Spec:** `docs/superpowers/specs/2026-06-22-m3-2-streams-design.md`
**Extends:** M0 + M1 + M2 + M3.1 (all merged on `main`)

These are the SHARED contracts other agents copy **verbatim** across Go, the Python worker, and
TypeScript. Names, JSON tags, signatures, SQL columns, and env vars below are authoritative.

**Codebase conventions reaffirmed (do not deviate):**
- Wire JSON is **snake_case** everywhere except the single legacy `authorizeUrl` field. M3.2 adds
  **no** camelCase fields (the source prompt's "camelCase DTOs" is wrong for this codebase).
- Strava IDs ARE activity IDs: `activities.strava_id` (PK), Go `int64`, TS `strava_id: number`.
- SQLite driver is `modernc.org/sqlite` (driver name `"sqlite"`); goose dialect `"sqlite3"`.
  `SetMaxOpenConns(1)` — single writer; syncs run sequentially.
- Sensor-absent values are pointers in Go (`*float64`/`*int64`) and `| null` in TS.
- Pure compute packages take slices + explicit `now` (no DB, no clock) for table tests; a thin
  DB-loading wrapper (`Engine`) calls them.

---

## 1. Migration `00006_m3_2_streams.sql`

Path: `/home/jake/project/help-my-run/backend/internal/store/migrations/00006_m3_2_streams.sql`.
Format mirrors `00005_m3_1_vo2max.sql` (goose Up/Down with StatementBegin/End). All three tables
FK to `activities(strava_id)` with `ON DELETE CASCADE` (matches `activity_splits`).

```sql
-- +goose Up
-- +goose StatementBegin
CREATE TABLE activity_streams (
    activity_id INTEGER PRIMARY KEY,
    source      TEXT NOT NULL,           -- 'strava' | 'garmin'
    series_gz   BLOB NOT NULL,           -- gzipped JSON of the normalized series (§2)
    fetched_at  TEXT NOT NULL,           -- ISO-8601 UTC (RFC3339)
    FOREIGN KEY (activity_id) REFERENCES activities (strava_id) ON DELETE CASCADE
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE stream_analyses (
    activity_id       INTEGER PRIMARY KEY,
    time_in_zone_json TEXT NOT NULL,     -- JSON array of ZoneTime (§3); "[]" when no HR
    decoupling_pct    REAL,              -- nullable: null when not computable (no HR / too short)
    pa_hr_first       REAL,              -- nullable: first-half Pa:HR (speed-per-beat, m/beat)
    pa_hr_second      REAL,              -- nullable: second-half Pa:HR
    zones_json        TEXT NOT NULL,     -- snapshot of zone boundaries used (§3 ZoneBounds)
    has_hr            INTEGER NOT NULL,  -- 0|1: did the source stream carry HR
    computed_at       TEXT NOT NULL,     -- ISO-8601 UTC (RFC3339)
    FOREIGN KEY (activity_id) REFERENCES activities (strava_id) ON DELETE CASCADE
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE stream_fetch_log (
    source         TEXT PRIMARY KEY,     -- always 'strava' (the trickle source)
    cursor_time    TEXT,                 -- nullable: oldest start_time reached in the recent window
    last_run_at    TEXT,                 -- nullable: ISO-8601 UTC of last trickle attempt
    last_fetched   INTEGER NOT NULL DEFAULT 0, -- count fetched in last run
    total_fetched  INTEGER NOT NULL DEFAULT 0, -- cumulative streams fetched
    status         TEXT NOT NULL DEFAULT 'never', -- 'ok' | 'error' | 'rate_limited' | 'never'
    error          TEXT,                 -- nullable: last error (non-null only on error/rate_limited)
    rate_limited_until TEXT              -- nullable: ISO-8601 UTC; trickle resumes after this
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE stream_fetch_log;
DROP TABLE stream_analyses;
DROP TABLE activity_streams;
-- +goose StatementEnd
```

Notes:
- `has_hr` is stored as INTEGER 0/1 (modernc has no BOOL; matches `agent_enabled` pattern).
- `decoupling_pct`/`pa_hr_first`/`pa_hr_second` are nullable REAL — scan into `sql.NullFloat64`
  → `*float64` (the `GetAthleteProfile` pattern).
- `series_gz BLOB` is the first BLOB in the codebase: pass a Go `[]byte` to `Exec`, scan into
  `&[]byte` (or `&series` where `series []byte`).
- `store_test.go::TestOpenAndMigrate` `wantTables` must add `activity_streams`, `stream_analyses`,
  `stream_fetch_log`.

---

## 2. Stream ingestion contract

### 2.1 Normalized series JSON — **struct-of-arrays** (`{t,hr,v,dist}`)

**Chosen shape (justified):** parallel arrays, NOT array-of-points.

```json
{ "t": [0,1,2,3], "hr": [104,105,106,107], "v": [0.0,1.59,1.66,1.69], "dist": [0.0,2.9,5.6,8.4] }
```

Justification: (1) gzip compresses long runs of homogeneous numbers far better than repeated
`{"t":..,"hr":..}` keys (~20-50 KB/run target, spec §10); (2) it is index-aligned exactly like
Strava's `key_by_type` streams and FIT record arrays, so both sources map 1:1; (3) the engine
iterates by index, never by key. **Missing HR** is represented by `hr` being an **empty array
`[]`** (length 0) while `t/v/dist` are populated — the engine's "no HR" degraded state. (Per-sample
HR gaps mid-stream are not expected from either source; if present they are dropped at normalize
time so `hr` stays index-aligned or empty.)

Canonical Go type (package `streams`, file `backend/internal/streams/series.go`):

```go
// Series is the normalized per-sample stream, struct-of-arrays, index-aligned.
// t is elapsed seconds since start; v is m/s; dist is cumulative meters; hr is bpm.
// hr is empty (len 0) when the source stream carried no heart-rate sensor data.
type Series struct {
    T    []float64 `json:"t"`
    HR   []float64 `json:"hr"`
    V    []float64 `json:"v"`
    Dist []float64 `json:"dist"`
}

func (s Series) HasHR() bool { return len(s.HR) > 0 }
func (s Series) Len() int    { return len(s.T) }
```

### 2.2 gzip storage (Go)

New imports `compress/gzip` + `encoding/json` (first use in store). Helpers live in package
`streams` (pure, table-testable), the store just persists the `[]byte`.

```go
// CompressSeries marshals s to JSON and gzips it (best-compression).
func CompressSeries(s Series) ([]byte, error)
// DecompressSeries gunzips + unmarshals back to a Series.
func DecompressSeries(gz []byte) (Series, error)
```

Store methods (file `backend/internal/store/streams.go`):

```go
type ActivityStream struct {
    ActivityID int64
    Source     string // "strava" | "garmin"
    SeriesGz   []byte
    FetchedAt  string // RFC3339 UTC
}

func (s *Store) UpsertActivityStream(as ActivityStream) error   // INSERT ... ON CONFLICT(activity_id) DO UPDATE
func (s *Store) GetActivityStream(activityID int64) (ActivityStream, error) // ErrNotFound if absent
func (s *Store) HasActivityStream(activityID int64) (bool, error)
```

`FetchedAt` set server-side: `time.Now().UTC().Format(time.RFC3339)` (UpsertActivity pattern).

### 2.3 Strava `GetActivityStreams` (package `strava`)

Add to `backend/internal/strava/types.go`:

```go
// Stream is one Strava stream channel (key_by_type=true response value).
type Stream struct {
    Type         string    `json:"type"`
    Data         []float64 `json:"data"`         // heartrate/distance/velocity/time all decode as float64
    SeriesType   string    `json:"series_type"`  // "time" | "distance"
    OriginalSize int       `json:"original_size"`
    Resolution   string    `json:"resolution"`   // "low" | "medium" | "high"
}

// StreamSet is the key_by_type=true response: keys are stream types
// ("time","heartrate","velocity_smooth","distance"). A missing-HR run OMITS the
// "heartrate" key entirely (not a null/empty array) — mirrors *float64 absence.
type StreamSet map[string]Stream
```

Add to `backend/internal/strava/client.go`:

```go
// streamKeys is the fixed CSV requested for M3.2 (spec §4).
const streamKeys = "time,heartrate,velocity_smooth,distance"

// GetActivityStreams fetches the per-sample streams for an activity, keyed by type.
// Path: /api/v3/activities/{id}/streams?keys=...&key_by_type=true
func (c *Client) GetActivityStreams(ctx context.Context, accessToken string, activityID int64) (StreamSet, error)
```

- Builds: `/api/v3/activities/<id>/streams?keys=time,heartrate,velocity_smooth,distance&key_by_type=true`
  (use `url.Values`; `keys` and `key_by_type` are both required).
- Reuses the authed-GET pattern but through the **new 429-aware path** (§2.4), not the bare
  `getJSON` (which has no backoff). Scope `activity:read_all` is already requested — no change.
- OAuth: same `accessToken` threading as `ListLaps`.

**Normalization Strava→Series** (package `streams`, file `streams/strava.go`):

```go
// FromStravaStreams maps a Strava StreamSet to the normalized Series.
// t   <- streams["time"].Data (elapsed seconds)
// v   <- streams["velocity_smooth"].Data (m/s)
// dist<- streams["distance"].Data (cumulative meters)
// hr  <- streams["heartrate"].Data, or [] when the "heartrate" key is absent.
// Arrays are truncated to the shortest present length to stay index-aligned.
func FromStravaStreams(ss strava.StreamSet) streams.Series
```

Missing-HR rule: `_, ok := ss["heartrate"]; if !ok { series.HR = nil }`.

### 2.4 Strava 429 backoff — **NEW (does not exist in M0)**

Build it in the strava package, self-contained (keeps base-URL-injectable httptest pattern).

```go
// ErrRateLimited is returned by GetActivityStreams on HTTP 429. RetryAfter is the
// time the 15-min window resets (next quarter-hour boundary :00/:15/:30/:45 UTC).
type ErrRateLimited struct {
    RetryAfter   time.Time // when to resume
    ReadUsage    string    // raw X-ReadRateLimit-Usage header ("15min,daily")
    ReadLimit    string    // raw X-ReadRateLimit-Limit header
}
func (e *ErrRateLimited) Error() string
```

- On `resp.StatusCode == 429`: parse `X-ReadRateLimit-Limit` / `X-ReadRateLimit-Usage` headers,
  compute `RetryAfter = next quarter-hour boundary`, return `*ErrRateLimited`. The orchestrator
  (§7) catches it, records `rate_limited_until` in `stream_fetch_log`, and pauses.
- Non-200, non-429 → existing error format: `fmt.Errorf("strava GET %s: status %d: %s", ...)`.
- The orchestrator may also use `errors.As(err, &rl)` to branch.

### 2.5 Garmin `.FIT` fallback — invoked ONLY when Strava stream has no HR

**FIT parser dependency (NEW):** add `garmin-fit-sdk` to
`/home/jake/project/help-my-run/garmin-worker/requirements.txt` (official Garmin SDK). `zipfile`
+ `io` are stdlib.

**Worker CLI subcommand** (`garmin_worker/cli.py`, mirror `fetch` discipline — single JSON object
on stdout, diagnostics to stderr, exit 0/non-zero):

```
worker.py stream --activity-id <garmin_activity_id> [--dry-run]
```

`--activity-id` is **Garmin's** activity id (not the Strava id). See §7 for the id-mapping flag.

**Client method** (`garmin_worker/client.py`, the only garminconnect importer):

```python
def download_activity_original(self, activity_id: str) -> bytes:
    """Download the ORIGINAL uploaded file (a ZIP of the .fit) for an activity."""
    from garminconnect import ActivityDownloadFormat
    return self._g.download_activity(activity_id, ActivityDownloadFormat.ORIGINAL)
```

**Normalizer** (`garmin_worker/normalize.py`, pure):

```python
def normalize_fit_stream(fit_bytes: bytes) -> dict:
    """Parse FIT record_mesgs → the §2.6 stream JSON object.
    Units already match Strava: speed/enhanced_speed in m/s, distance in meters.
    t = (timestamp - first_timestamp).total_seconds(); hr omitted-per-record → []."""
```

FIT field mapping (per record message): `timestamp` → `t` (elapsed secs), `heart_rate` → `hr`
(bpm; `None`/absent → drop, leaving `hr=[]` if no record has HR), `enhanced_speed` (fallback
`speed`) → `v` (m/s), `distance` → `dist` (m).

### 2.6 Worker stream output JSON (stdout)

Single object; consumed by the Go runner `RunGarminFetchFIT` (§ below). Key order fixed.

```json
{
  "activity_id": 14820001234,
  "source": "garmin",
  "fetched_at": "2026-06-22T05:00:12Z",
  "series": { "t": [0,1,2], "hr": [104,105,106], "v": [0.0,1.59,1.66], "dist": [0.0,2.9,5.6] }
}
```

`activity_id` here echoes back the **Strava** activity id the Go caller passed in via env/flag
(so the store row keys correctly); the Garmin download id is resolved by the caller (§7). The
worker emits whatever id it was told to echo (CLI `--activity-id` is the Garmin download id;
the echoed `activity_id` field is a separate `--echo-id` flag — see §7 mapping). `series.hr` is
`[]` when the FIT had no HR (degraded state preserved).

**Go runner** (`backend/internal/garmin/runner.go`, new method + type in `garmin/types.go`):

```go
// FITStreamOutput is the worker `stream` subcommand stdout JSON.
type FITStreamOutput struct {
    ActivityID int64           `json:"activity_id"`
    Source     string          `json:"source"`      // "garmin"
    FetchedAt  string          `json:"fetched_at"`
    Series     FITSeries       `json:"series"`
}
type FITSeries struct {
    T    []float64 `json:"t"`
    HR   []float64 `json:"hr"`
    V    []float64 `json:"v"`
    Dist []float64 `json:"dist"`
}

func (r Runner) RunGarminFetchFIT(ctx context.Context, garminActivityID, echoActivityID int64, extraEnv []string) (*FITStreamOutput, error)
```

Same exec/stderr-capture/JSON-parse shape as `RunGarminFetch`. `FITSeries` is structurally
identical to `streams.Series` and converts directly.

---

## 3. Streams engine Go types + algorithms

Package `streams`, path `backend/internal/streams/`. Pure functions, table-driven tests
(`streams_test.go`). No DB, no clock.

### 3.1 Zone boundaries — source + documented defaults

Zones derived from `store.AthleteProfile.{Zone2CeilingBpm, ThresholdBpm, MaxHRBpm}` (all
`*int64`, nullable). **5 HR zones** with these boundaries (inclusive-low, exclusive-high; top
zone open-ended):

| Zone | Range (bpm) | Meaning |
|---|---|---|
| Z1 | `< z1Hi` | recovery |
| Z2 | `[z1Hi, z2Hi)` | aerobic / easy |
| Z3 | `[z2Hi, z3Hi)` | tempo |
| Z4 | `[z3Hi, z4Hi)` | threshold |
| Z5 | `>= z4Hi` | VO2max |

Boundary derivation (documented defaults when profile fields unset, mirroring `defaultRefHRBpm`):

```go
// Documented fallbacks (used only when the corresponding profile field is nil).
const (
    DefaultMaxHRBpm   = 190.0 // when MaxHRBpm nil
    DefaultZone2Hi    = 145.0 // when Zone2CeilingBpm nil (matches progress.defaultRefHRBpm)
    DefaultThreshold  = 170.0 // when ThresholdBpm nil
)

// ZoneBounds are the 4 internal boundaries (z1Hi..z4Hi) used for a computation.
// Snapshotted into stream_analyses.zones_json so a profile change triggers recompute.
type ZoneBounds struct {
    Z1Hi float64 `json:"z1_hi"` // Z1->Z2 boundary
    Z2Hi float64 `json:"z2_hi"` // Z2->Z3 (zone2 ceiling)
    Z3Hi float64 `json:"z3_hi"` // Z3->Z4
    Z4Hi float64 `json:"z4_hi"` // Z4->Z5 (threshold)
}

// ZonesFromProfile derives the 5-zone boundaries from the profile + defaults.
// z2Hi = Zone2CeilingBpm (or DefaultZone2Hi)
// z4Hi = ThresholdBpm    (or DefaultThreshold)
// maxHR= MaxHRBpm        (or DefaultMaxHRBpm)
// z1Hi = round(0.60*z2Hi/0.75) heuristic -> documented as 0.80*z2Hi   [see note]
// z3Hi = midpoint(z2Hi, z4Hi)
func ZonesFromProfile(p store.AthleteProfile) ZoneBounds
```

Note for implementer: `z1Hi` and `z3Hi` are derived, not in the profile. Lock them as:
`z1Hi = 0.80 * z2Hi` (recovery ceiling) and `z3Hi = (z2Hi + z4Hi) / 2` (tempo midpoint). These
are documented heuristics; `maxHR` bounds Z5's notional top but Z5 is open-ended for bucketing.

### 3.2 Time-in-zone

```go
// ZoneTime is one HR zone's dwell time + share of moving time. JSON is the
// stored time_in_zone_json element AND the wire DTO element (snake_case).
type ZoneTime struct {
    Zone    int     `json:"zone"`    // 1..5
    Seconds float64 `json:"seconds"` // dwell time in this zone
    Pct     float64 `json:"pct"`     // 0..100 share of total HR-sampled seconds
}

// TimeInZone buckets each sample's HR into a zone, accumulating dt between
// consecutive t[] samples (dt defaults to 1.0s for per-second streams; for the
// last sample, reuse the prior dt). Returns exactly 5 ZoneTime entries (Z1..Z5),
// pct relative to summed dt. Empty/no-HR series -> empty slice (len 0).
func TimeInZone(s Series, zb ZoneBounds) []ZoneTime
```

- Sample i's zone uses `s.HR[i]`; dwell = `t[i+1]-t[i]` (last sample reuses previous dt).
- `pct = seconds / totalSeconds * 100`; if totalSeconds==0 → all pct 0.
- No HR (`!s.HasHR()`) → return `[]ZoneTime{}` (stored as `"[]"`).

### 3.3 Decoupling (Pa:HR drift)

```go
// Decoupling is the aerobic-durability drift result.
type Decoupling struct {
    DecouplingPct *float64 `json:"decoupling_pct"` // nil when not computable
    PaHRFirst     *float64 `json:"pa_hr_first"`    // first-half speed-per-beat (m/beat), nil if N/A
    PaHRSecond    *float64 `json:"pa_hr_second"`   // second-half, nil if N/A
}

// ComputeDecoupling splits the series at the MOVING-TIME MIDPOINT (half of total
// elapsed t span), computes Pa:HR = mean(v)/mean(hr) for each half, and the drift:
//   decoupling_pct = (paHRFirst - paHRSecond) / paHRFirst * 100
// Higher drift (HR rises relative to pace in the 2nd half) = worse durability,
// so lower_is_better. Returns all-nil when: no HR, < 2 samples per half,
// mean(hr)==0 in a half, or paHRFirst==0.
func ComputeDecoupling(s Series) Decoupling
```

**Definitions locked:**
- Split point = `tMid = (t[0] + t[last]) / 2`; samples with `t <= tMid` are first half, rest second.
- `PaHR = mean(v over half) / mean(hr over half)` (meters per beat per second; units cancel in the
  ratio, so the absolute value is small ~0.01-0.03 m/beat — this is fine, only the % drift matters).
- `decoupling_pct = (paHRFirst - paHRSecond) / paHRFirst * 100`. Positive = HR drifted up / pace
  held = aerobic decoupling. Sub-5% on an easy long run = good (spec §8 plain-language read).

### 3.4 StreamAnalysis aggregate (engine output → store + DTO)

```go
// StreamAnalysis is the cached per-run analysis (one stream_analyses row).
type StreamAnalysis struct {
    ActivityID    int64      `json:"activity_id"`
    HasHR         bool       `json:"has_hr"`
    TimeInZone    []ZoneTime `json:"time_in_zone"`     // [] when !HasHR
    DecouplingPct *float64   `json:"decoupling_pct"`
    PaHRFirst     *float64   `json:"pa_hr_first"`
    PaHRSecond    *float64   `json:"pa_hr_second"`
    Zones         ZoneBounds `json:"zones"`            // boundaries used (snapshot)
    ComputedAt    string     `json:"computed_at"`      // RFC3339 UTC
}

// Analyze runs TimeInZone + ComputeDecoupling over a decompressed Series with the
// given zone boundaries. Pure; the caller sets ActivityID + ComputedAt.
func Analyze(activityID int64, s Series, zb ZoneBounds) StreamAnalysis
```

Store struct + methods (`backend/internal/store/streams.go`) — note `time_in_zone_json` and
`zones_json` are persisted as marshaled strings, unmarshaled on read:

```go
type StreamAnalysisRow struct {
    ActivityID      int64
    TimeInZoneJSON  string   // marshaled []ZoneTime
    DecouplingPct   *float64
    PaHRFirst       *float64
    PaHRSecond      *float64
    ZonesJSON       string   // marshaled ZoneBounds
    HasHR           bool
    ComputedAt      string
}
func (s *Store) UpsertStreamAnalysis(r StreamAnalysisRow) error          // ON CONFLICT(activity_id) DO UPDATE
func (s *Store) GetStreamAnalysis(activityID int64) (StreamAnalysisRow, error) // ErrNotFound if absent
func (s *Store) ListStreamAnalyses(limit int) ([]StreamAnalysisRow, error)     // for the progress series (§5)
```

---

## 4. Analysis cache + recompute

The cache snapshots the zones used (`zones_json` / `ZoneBounds`) so a profile-zone change
triggers a recompute **from stored raw** (no re-fetch). Logic lives in the streams DB-loading
wrapper `Engine` (`backend/internal/streams/engine.go`).

```go
type Engine struct { store *store.Store }
func New(s *store.Store) *Engine

// GetOrComputeAnalysis returns the cached StreamAnalysis, recomputing from the
// stored raw stream when the cached zones_json differs from the CURRENT profile
// zones. Returns ErrNotFound (or a not-fetched sentinel) when no stream is stored.
func (e *Engine) GetOrComputeAnalysis(ctx context.Context, activityID int64) (streams.StreamAnalysis, error)

// FetchAndAnalyze (on-demand / trickle): fetch the stream (Strava primary; Garmin
// FIT fallback when Strava lacks HR), store it gzipped, compute + cache the
// analysis, and return it.
func (e *Engine) FetchAndAnalyze(ctx context.Context, activityID int64) (streams.StreamAnalysis, error)
```

**Recompute rule (exact):** on `GetOrComputeAnalysis`, load the profile, compute
`current := streams.ZonesFromProfile(profile)`; load the cached row; if `cached.ZonesJSON !=
marshal(current)`, decompress `activity_streams.series_gz`, re-run `streams.Analyze` with
`current`, upsert the new row (new `computed_at`), return it. Else return the cached row decoded.
No `activity_streams` re-fetch ever occurs on a zone change.

`api.Streams` seam interface (mirrors `api.Progress`):

```go
// Streams is the M3.2 engine seam injected from main.go. *streams.Engine satisfies it.
type Streams interface {
    GetOrComputeAnalysis(ctx context.Context, activityID int64) (streams.StreamAnalysis, error)
    FetchAndAnalyze(ctx context.Context, activityID int64) (streams.StreamAnalysis, error)
}
```

Add `Streams Streams` to `api.Deps`; wire `streamsEngine := streams.New(s)` (+ strava client +
garmin runner deps it needs) in `main.go::Wire` and pass `Streams: streamsEngine`.

---

## 5. Decoupling signal for M3.1 progress

Add to `backend/internal/progress/progress.go` (verbatim const + meta + builder; mirror
`paceAtHRSeries`):

```go
const SignalDecoupling = "decoupling" // per-run Pa:HR drift %, weekly-median over the window

// in signalMetas:
SignalDecoupling: {label: "Decoupling", unit: "%", lowerIsBetter: true, isPace: false},
```

New series builder — reads per-run `decoupling_pct` joined to the activity's `start_time` for
bucketing, weekly-median per bucket (gap when none):

```go
// decouplingSeries builds per-bucket median decoupling % from stream analyses.
// analyses pairs an activity start_time with its decoupling_pct (nil dropped).
// A bucket with no qualifying run -> nil (gap). Lower = better.
func decouplingSeries(analyses []StreamAnalysisPoint, buckets []weekBucket) []*float64

// StreamAnalysisPoint is the minimal progress input: a run's start_time + decoupling.
type StreamAnalysisPoint struct {
    StartTime     string   // activity start_time (RFC3339), for bucketing via metrics.ParseStart
    DecouplingPct *float64 // nil -> skipped
}
```

**`ComputeProgress` signature change (unavoidable):** add `streamPts []StreamAnalysisPoint`
before `weeks`:

```go
func ComputeProgress(
    acts []store.Activity,
    recovery []store.RecoveryDay,
    vo2max []store.Vo2maxPoint,
    streamPts []StreamAnalysisPoint,
    profile store.AthleteProfile,
    weeks int,
    now time.Time,
) ProgressReport
```

Append to the `signals` slice (after `weekly_load`, keeping it last in display order is a choice —
**lock decoupling as the LAST signal**, index 5):

```go
buildSignal(SignalDecoupling, decouplingSeries(streamPts, buckets)),
```

**`enough_data` gate decision (LOCKED):** decoupling is a real fitness signal and **counts toward
the gate** (only `weekly_load` is excluded). So the gate loop's `if s.Key == SignalWeeklyLoad`
skip is unchanged; decoupling participates like the other four. `enoughDataMinSignals = 2`
unchanged.

**`Engine.Report` change** (`progress/engine.go`): before `ComputeProgress`, load stream analyses:

```go
saRows, err := e.store.ListStreamAnalyses(activityLimit(weeks)) // reuse the same cap
// map saRows -> []StreamAnalysisPoint by joining activity_id to its start_time
// (Report already has `acts` in memory; build an id->start_time map).
```

Then pass `streamPts` into `ComputeProgress`. `progress_test.go` / `engine_test.go` gain a
`streamPts` arg (empty slice where decoupling is not under test).

The app needs **no** new progress code: `progress.tsx` renders one `TrendCard` per signal, and the
TS `TrendSummary.key` is already an open `string`. The card `progress-card-decoupling` auto-emits.

---

## 6. REST API

Add inside the bearer-protected group in `router.go` (after the M3.1 routes):

```go
r.Post("/api/activities/{id}/stream/fetch", h.fetchStream)
r.Get("/api/activities/{id}/analysis", h.activityAnalysis)
```

Path-param handling (first `chi.URLParam` use; needs `import "github.com/go-chi/chi/v5"`):

```go
idStr := chi.URLParam(r, "id")
id, err := strconv.ParseInt(idStr, 10, 64)
if err != nil { writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"}); return }
```

Handlers file: `backend/internal/api/stream_handlers.go`.

### 6.1 Wire DTO (snake_case) — `dto.go`

```go
type zoneTimeDTO struct {
    Zone    int     `json:"zone"`
    Seconds float64 `json:"seconds"`
    Pct     float64 `json:"pct"`
}
type zoneBoundsDTO struct {
    Z1Hi float64 `json:"z1_hi"`
    Z2Hi float64 `json:"z2_hi"`
    Z3Hi float64 `json:"z3_hi"`
    Z4Hi float64 `json:"z4_hi"`
}
type streamAnalysisDTO struct {
    ActivityID    int64         `json:"activity_id"`
    HasStream     bool          `json:"has_stream"`       // true once a raw stream is stored
    HasHR         bool          `json:"has_hr"`           // false -> "no HR in this stream" state
    TimeInZone    []zoneTimeDTO `json:"time_in_zone"`     // [] when !has_hr
    DecouplingPct *float64      `json:"decoupling_pct"`   // null when not computable
    PaHRFirst     *float64      `json:"pa_hr_first"`
    PaHRSecond    *float64      `json:"pa_hr_second"`
    Zones         zoneBoundsDTO `json:"zones"`
    Source        string        `json:"source"`           // "strava" | "garmin"
    ComputedAt    string        `json:"computed_at"`
}
```

### 6.2 `GET /api/activities/{id}/analysis`

- Auth: bearer (in the protected group). 401 `{"error":"unauthorized"}` on bad token.
- Calls `h.d.Streams.GetOrComputeAnalysis(ctx, id)`.
- **Not-fetched contract (LOCKED): return HTTP 200 with `{"has_stream": false, ...}`** (NOT 404).
  Rationale: it lets the app branch on a field, and the empty state is a normal state, not an
  error. (The app research flags this exact decision — locking 200+`has_stream:false`.)

Not-fetched 200 body:
```json
{
  "activity_id": 14820001234,
  "has_stream": false,
  "has_hr": false,
  "time_in_zone": [],
  "decoupling_pct": null,
  "pa_hr_first": null,
  "pa_hr_second": null,
  "zones": { "z1_hi": 116, "z2_hi": 145, "z3_hi": 157.5, "z4_hi": 170 },
  "source": "",
  "computed_at": ""
}
```

Fetched + HR example (200):
```json
{
  "activity_id": 14820001234,
  "has_stream": true,
  "has_hr": true,
  "time_in_zone": [
    { "zone": 1, "seconds": 120, "pct": 4.0 },
    { "zone": 2, "seconds": 2400, "pct": 80.0 },
    { "zone": 3, "seconds": 480, "pct": 16.0 },
    { "zone": 4, "seconds": 0, "pct": 0.0 },
    { "zone": 5, "seconds": 0, "pct": 0.0 }
  ],
  "decoupling_pct": 4.2,
  "pa_hr_first": 0.0212,
  "pa_hr_second": 0.0203,
  "zones": { "z1_hi": 116, "z2_hi": 145, "z3_hi": 157.5, "z4_hi": 170 },
  "source": "strava",
  "computed_at": "2026-06-22T07:00:00Z"
}
```

Fetched, no HR (200):
```json
{
  "activity_id": 14820001234,
  "has_stream": true,
  "has_hr": false,
  "time_in_zone": [],
  "decoupling_pct": null,
  "pa_hr_first": null,
  "pa_hr_second": null,
  "zones": { "z1_hi": 116, "z2_hi": 145, "z3_hi": 157.5, "z4_hi": 170 },
  "source": "strava",
  "computed_at": "2026-06-22T07:00:00Z"
}
```

### 6.3 `POST /api/activities/{id}/stream/fetch`

- Auth: bearer. **No request body** (empty body OK).
- Calls `h.d.Streams.FetchAndAnalyze(ctx, id)`: fetch-if-missing, compute, cache, return.
- Success → 200, the SAME `streamAnalysisDTO` (with `has_stream:true`).
- Strava 429 surfaced → 429 `{"error":"rate_limited"}` (so the app can retry later).
- Other fetch failure → 500 `{"error": "..."}`.

### 6.4 `GET /api/progress`

Unchanged route/handler; the report's `signals` array now carries the `decoupling` entry (§5).

---

## 7. Fetch orchestration contract

Lives in `backend/internal/sync/` (new `streams_sync.go`) + the streams `Engine`. Invoked from
`SyncStrava` after the activity/laps upsert loop (single SQLite writer; runs before Garmin in
`SyncAll`).

### 7.1 Recent window + budget (config-driven, §9)

```go
// Recent window: runs with start_time within the last StreamRecentWeeks weeks that
// LACK an activity_streams row. Default 12 weeks (matches garminBackfillDays=84d).
// Per-sync budget: fetch at most StreamFetchBudget streams, oldest-eligible-first,
// trickled. Remaining are picked up next sync (resumable).
```

### 7.2 `stream_fetch_log` semantics (resumable)

- One row, `source = "strava"`.
- `cursor_time`: oldest activity `start_time` already attempted in the recent window. Next sync
  resumes from runs newer than nothing / older than cursor as appropriate — selection query is
  "recent-window runs without a stream, ordered by start_time DESC, LIMIT budget".
- `status`: `"ok"` (budget done or window exhausted), `"rate_limited"` (hit 429; set
  `rate_limited_until = ErrRateLimited.RetryAfter`), `"error"` (other), `"never"`.
- `rate_limited_until`: while `now < rate_limited_until`, the trickle is SKIPPED entirely this
  sync (cheap pause; no Strava call). Cleared on the next successful run.
- `last_fetched` / `total_fetched`: counters for observability.
- Mirrors `sync_log` upsert style (`GetSyncLog`/`UpdateSyncLog`) — provide
  `GetStreamFetchLog()` / `UpdateStreamFetchLog(StreamFetchLog)` in `store/streams.go`:

```go
type StreamFetchLog struct {
    Source           string
    CursorTime       *string
    LastRunAt        *string
    LastFetched      int64
    TotalFetched     int64
    Status           string
    Error            *string
    RateLimitedUntil *string
}
```

### 7.3 429 backoff reuse

The trickle calls `Engine.FetchAndAnalyze` per run; on `*strava.ErrRateLimited`
(`errors.As`), it stops the loop, writes `status="rate_limited"` + `rate_limited_until`, and
returns (does not error the whole Strava sync). On-demand `POST .../stream/fetch` surfaces 429 to
the client (§6.3).

### 7.4 Garmin FIT fallback trigger + id mapping

- Within `FetchAndAnalyze`: fetch Strava streams; `FromStravaStreams`; if `!series.HasHR()` AND a
  Garmin id is resolvable → call `RunGarminFetchFIT` and prefer its series if it has HR.
- **Strava↔Garmin id mapping (FLAG — not in the spec data model):** `download_activity` needs the
  **Garmin** activity id, but `activity_streams.activity_id` is the **Strava** id. M3.2 has no
  mapping table. **Locked decision for v1:** the Garmin FIT fallback is **best-effort and
  attempted only when a mapping is available**; absent a mapping the run degrades to "no HR" (store
  the Strava raw, `has_hr:false`). Implementation may add a lightweight `external_id`-based match
  (Strava `external_id` often encodes the Garmin file name) later; **do not block M3.2 on it.**
  The Go runner signature already separates `garminActivityID` (download) from `echoActivityID`
  (Strava PK) for when the mapping exists.

---

## 8. App TS types + hooks

### 8.1 Types (`app/src/api/types.ts`, append after M3.1 block, snake_case)

```ts
// --- M3.2 streams types (snake_case wire JSON; mirror the Go DTO exactly) ---

export interface ZoneTime {
  zone: number;     // 1..5
  seconds: number;  // dwell time in zone
  pct: number;      // 0..100 share of HR-sampled seconds
}
export interface ZoneBounds {
  z1_hi: number;
  z2_hi: number;
  z3_hi: number;
  z4_hi: number;
}
export interface StreamAnalysis {
  activity_id: number;
  has_stream: boolean;            // false -> offer "Fetch stream"
  has_hr: boolean;                // false -> "No HR in this stream"
  time_in_zone: ZoneTime[];       // [] when !has_hr
  decoupling_pct: number | null;  // null when not computable
  pa_hr_first: number | null;
  pa_hr_second: number | null;
  zones: ZoneBounds;
  source: 'strava' | 'garmin' | '';
  computed_at: string;            // '' when not fetched
}
```

### 8.2 Hooks (`app/src/api/hooks.ts`)

```ts
export function useActivityAnalysis(activityId: number) {
  return useQuery({
    queryKey: ['analysis', activityId],
    queryFn: () => apiGet<StreamAnalysis>(`/api/activities/${activityId}/analysis`),
    enabled: Number.isFinite(activityId),
    // GET returns 200 + { has_stream:false } when not fetched (§6.2), so no 404 branch needed.
  });
}

export function useFetchStream(activityId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => apiPost<StreamAnalysis>(`/api/activities/${activityId}/stream/fetch`),
    onSuccess: (data) => {
      queryClient.setQueryData(['analysis', activityId], data);
      queryClient.invalidateQueries({ queryKey: ['progress'] }); // decoupling signal may now fill
    },
  });
}
```

Import `StreamAnalysis` in the hooks type-import block. `apiPost` already sends no body when none
passed.

### 8.3 Run-detail route

- New file `app/app/run/[id].tsx`; param via `useLocalSearchParams<{ id: string }>()`,
  `activityId = Number(id)`.
- Register in `app/app/_layout.tsx` `<Stack>`: `<Stack.Screen name="run/[id]" options={{ title: 'Run detail' }} />`.
- Make recent-runs rows tappable in `app/app/index.tsx`: wrap the row in
  `<Link href={{ pathname: '/run/[id]', params: { id: String(item.strava_id) } }} asChild>`
  + `<Pressable testID={\`run-row-${item.strava_id}\`}>` (Pressable already imported).

### 8.4 Time-in-zone bars (pure RN `<View>`, no chart dep)

- One `ZoneBar` row per `ZoneTime`: label `Z{zone}`, a flex track with a `width: \`${pct}%\`` fill
  (brand orange `#fc4c02`, `overflow:'hidden'` track), and `{Math.round(seconds/60)} min · {pct.toFixed(0)}%`.
- testIDs (LOCKED for tests):
  - `zone-bar-{zone}` per bar.
  - `decoupling-value` — `{decoupling_pct == null ? '—' : decoupling_pct.toFixed(1) + '%'}`,
    colored via lower-is-better palette (green `#1b8a3a` / red `#c0392b`).
  - `run-no-hr` — shown when `has_stream && !has_hr` ("No HR in this stream"; no bars).
  - `btn-fetch-stream` — `<Pressable>` shown when `!has_stream`; disabled + "Fetching…" while
    `fetch.isPending` (mirror `btn-today-undo`).
- Decoupling plain-language read per spec §8 ("<5% on an easy long run = good durability").
- Progress decoupling card auto-renders via the generic `TrendCard` (testIDs
  `progress-card-decoupling`, `progress-arrow-decoupling`, `progress-spark-decoupling`); no new app
  component.

---

## 9. Config additions + full file tree

### 9.1 Config (`backend/internal/config/config.go`) — new fields

```go
// M3.2: stream fetch trickle.
StreamRecentWeeks int `envconfig:"STREAM_RECENT_WEEKS" default:"12"` // recent-window depth
StreamFetchBudget int `envconfig:"STREAM_FETCH_BUDGET" default:"10"` // max streams fetched per sync
```

(Garmin email/password/tokenstore + PythonBin/WorkerScript already exist and are reused for the
FIT subcommand — no new worker env vars.)

### 9.2 File tree

**Backend — new files**
- `backend/internal/store/migrations/00006_m3_2_streams.sql` — 3 tables (§1).
- `backend/internal/store/streams.go` — `ActivityStream`, `StreamAnalysisRow`, `StreamFetchLog` +
  CRUD (`UpsertActivityStream`, `GetActivityStream`, `HasActivityStream`, `UpsertStreamAnalysis`,
  `GetStreamAnalysis`, `ListStreamAnalyses`, `GetStreamFetchLog`, `UpdateStreamFetchLog`).
- `backend/internal/store/streams_test.go` — temp-DB CRUD + BLOB round-trip.
- `backend/internal/streams/series.go` — `Series`, `CompressSeries`, `DecompressSeries`.
- `backend/internal/streams/strava.go` — `FromStravaStreams`.
- `backend/internal/streams/zones.go` — `ZoneBounds`, `ZonesFromProfile`, defaults.
- `backend/internal/streams/analyze.go` — `ZoneTime`, `TimeInZone`, `Decoupling`,
  `ComputeDecoupling`, `StreamAnalysis`, `Analyze`.
- `backend/internal/streams/engine.go` — `Engine`, `New`, `GetOrComputeAnalysis`, `FetchAndAnalyze`.
- `backend/internal/streams/*_test.go` — table-driven engine tests (fixtures).
- `backend/internal/api/stream_handlers.go` — `fetchStream`, `activityAnalysis`.
- `backend/internal/api/stream_handlers_test.go` — httptest + fake `Streams` seam + auth/not-fetched.
- `backend/internal/sync/streams_sync.go` — recent-window trickle + `stream_fetch_log`.
- `backend/internal/strava/testdata/strava_streams.json` — recorded `key_by_type` fixture.

**Backend — modified files**
- `backend/internal/strava/types.go` — add `Stream`, `StreamSet`.
- `backend/internal/strava/client.go` — add `GetActivityStreams`, `streamKeys`, `ErrRateLimited`,
  429-aware GET path.
- `backend/internal/strava/client_test.go` — streams path/params + 429 backoff test.
- `backend/internal/garmin/types.go` — add `FITStreamOutput`, `FITSeries`.
- `backend/internal/garmin/runner.go` — add `RunGarminFetchFIT`.
- `backend/internal/garmin/runner_test.go` — stub-worker `stream` subcommand test.
- `backend/internal/progress/progress.go` — `SignalDecoupling`, meta, `decouplingSeries`,
  `StreamAnalysisPoint`, `ComputeProgress` signature + signal append + gate inclusion.
- `backend/internal/progress/engine.go` — load `ListStreamAnalyses`, build `streamPts`, pass through.
- `backend/internal/progress/progress_test.go`, `engine_test.go` — new `streamPts` arg.
- `backend/internal/api/dto.go` — add `zoneTimeDTO`, `zoneBoundsDTO`, `streamAnalysisDTO`.
- `backend/internal/api/router.go` — add the two `{id}` routes (protected group).
- `backend/internal/api/deps` (router.go) — add `Streams Streams` to `Deps` + the `Streams` seam.
- `backend/internal/config/config.go` — `StreamRecentWeeks`, `StreamFetchBudget`.
- `backend/cmd/server/main.go` — construct `streams.New(...)`, inject `Streams:`, call trickle.
- `backend/internal/store/store_test.go` — `wantTables` += 3 stream tables.
- `backend/internal/sync/sync.go` — call the trickle after the Strava upsert loop.

**Worker — new/modified**
- `garmin-worker/requirements.txt` — add `garmin-fit-sdk` (MODIFIED; first FIT dep).
- `garmin-worker/garmin_worker/cli.py` — add `stream` subcommand (`--activity-id`, `--echo-id`,
  `--dry-run`).
- `garmin-worker/garmin_worker/client.py` — add `download_activity_original`.
- `garmin-worker/garmin_worker/normalize.py` — add `normalize_fit_stream`.
- `garmin-worker/garmin_worker/fetcher.py` — add a FIT-fetch orchestration fn (mirror `run_fetch`).
- `garmin-worker/tests/fixtures/sample.fit` — fixture FIT (NEW).
- `garmin-worker/tests/test_fit_stream.py` — normalize FIT → §2.6 JSON (NEW).

**App — new/modified**
- `app/app/run/[id].tsx` — run-detail screen (NEW).
- `app/app/__tests__/run-detail.test.tsx` — bars / decoupling / no-HR / fetch states (NEW).
- `app/app/_layout.tsx` — add `run/[id]` Stack.Screen.
- `app/app/index.tsx` — wrap recent-runs row in `<Link>`/`<Pressable testID="run-row-...">`.
- `app/app/__tests__/index.test.tsx` — extend expo-router mock (`useLocalSearchParams`), assert row testID.
- `app/src/api/types.ts` — add `ZoneTime`, `ZoneBounds`, `StreamAnalysis`.
- `app/src/api/hooks.ts` — add `useActivityAnalysis`, `useFetchStream`.

---

## Locked decisions (resolving the research flags)

1. **Series shape** = struct-of-arrays `{t,hr,v,dist}` (gzip-friendly, index-aligned). Missing HR
   = empty `hr` array.
2. **Strava 429 backoff is NEW** — built in the strava package as `ErrRateLimited` + quarter-hour
   reset; orchestrator pauses via `stream_fetch_log.rate_limited_until`.
3. **FIT dep** = `garmin-fit-sdk` added to `requirements.txt`.
4. **Not-fetched API response** = HTTP **200** + `has_stream:false` (NOT 404). App branches on the
   field; `useActivityAnalysis` needs no 404 special-casing.
5. **Decoupling COUNTS toward `enough_data`** (only `weekly_load` excluded); `enoughDataMinSignals`
   stays 2.
6. **Decoupling is the LAST signal** (index 5) in the `signals` slice.
7. **Zone count = 5**, boundaries from profile `Zone2CeilingBpm` (z2_hi), `ThresholdBpm` (z4_hi),
   `MaxHRBpm`; derived `z1_hi=0.80*z2_hi`, `z3_hi=(z2_hi+z4_hi)/2`; documented defaults 145/170/190.
8. **Decoupling math:** split at moving-time midpoint `tMid=(t[0]+t[last])/2`; `PaHR=mean(v)/mean(hr)`
   per half; `decoupling_pct=(first-second)/first*100`; all-nil when no HR / too short / zero means.
9. **Garmin↔Strava id mapping is unresolved** — FIT fallback is best-effort, attempted only when a
   Garmin id is resolvable; absent it, degrade to "no HR". Not a blocker for M3.2.
```
