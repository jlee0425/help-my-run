package store

// SleepRow maps to garmin_sleep.
type SleepRow struct {
	Date      string
	DurationS *int64
	DeepS     *int64
	LightS    *int64
	RemS      *int64
	AwakeS    *int64
	Score     *int64
	RawJSON   string
}

// HrvRow maps to garmin_hrv.
type HrvRow struct {
	Date           string
	LastNightAvgMs *int64
	Status         *string
	RawJSON        string
}

// BodyBatteryRow maps to garmin_body_battery.
type BodyBatteryRow struct {
	Date    string
	Charged *int64
	Drained *int64
	High    *int64
	Low     *int64
	RawJSON string
}

// RhrRow maps to garmin_rhr.
type RhrRow struct {
	Date      string
	RestingHR *int64
	RawJSON   string
}

// SleepFields is the recovery sub-record for sleep (no date/raw_json).
type SleepFields struct {
	DurationS *int64
	DeepS     *int64
	LightS    *int64
	RemS      *int64
	AwakeS    *int64
	Score     *int64
}

// HrvFields is the recovery sub-record for hrv.
type HrvFields struct {
	LastNightAvgMs *int64
	Status         *string
}

// BodyBatteryFields is the recovery sub-record for body battery.
type BodyBatteryFields struct {
	Charged *int64
	Drained *int64
	High    *int64
	Low     *int64
}

// RhrFields is the recovery sub-record for resting HR.
type RhrFields struct {
	RestingHR *int64
}

// RecoveryDay is one merged calendar date across the four garmin_* tables.
// Any sub-record is nil when that source has no data for the date.
type RecoveryDay struct {
	Date        string
	Sleep       *SleepFields
	HRV         *HrvFields
	BodyBattery *BodyBatteryFields
	RHR         *RhrFields
}

// UpsertSleep upserts one garmin_sleep row by date.
func (s *Store) UpsertSleep(r SleepRow) error {
	_, err := s.DB.Exec(`
		INSERT INTO garmin_sleep (date, duration_s, deep_s, light_s, rem_s, awake_s, score, raw_json)
		VALUES (?,?,?,?,?,?,?,?)
		ON CONFLICT(date) DO UPDATE SET
			duration_s=excluded.duration_s, deep_s=excluded.deep_s, light_s=excluded.light_s,
			rem_s=excluded.rem_s, awake_s=excluded.awake_s, score=excluded.score,
			raw_json=excluded.raw_json`,
		r.Date, r.DurationS, r.DeepS, r.LightS, r.RemS, r.AwakeS, r.Score, r.RawJSON)
	return err
}

// UpsertHrv upserts one garmin_hrv row by date.
func (s *Store) UpsertHrv(r HrvRow) error {
	_, err := s.DB.Exec(`
		INSERT INTO garmin_hrv (date, last_night_avg_ms, status, raw_json)
		VALUES (?,?,?,?)
		ON CONFLICT(date) DO UPDATE SET
			last_night_avg_ms=excluded.last_night_avg_ms, status=excluded.status,
			raw_json=excluded.raw_json`,
		r.Date, r.LastNightAvgMs, r.Status, r.RawJSON)
	return err
}

// UpsertBodyBattery upserts one garmin_body_battery row by date.
func (s *Store) UpsertBodyBattery(r BodyBatteryRow) error {
	_, err := s.DB.Exec(`
		INSERT INTO garmin_body_battery (date, charged, drained, high, low, raw_json)
		VALUES (?,?,?,?,?,?)
		ON CONFLICT(date) DO UPDATE SET
			charged=excluded.charged, drained=excluded.drained, high=excluded.high,
			low=excluded.low, raw_json=excluded.raw_json`,
		r.Date, r.Charged, r.Drained, r.High, r.Low, r.RawJSON)
	return err
}

// UpsertRhr upserts one garmin_rhr row by date.
func (s *Store) UpsertRhr(r RhrRow) error {
	_, err := s.DB.Exec(`
		INSERT INTO garmin_rhr (date, resting_hr, raw_json)
		VALUES (?,?,?)
		ON CONFLICT(date) DO UPDATE SET
			resting_hr=excluded.resting_hr, raw_json=excluded.raw_json`,
		r.Date, r.RestingHR, r.RawJSON)
	return err
}

// Vo2maxRow maps to garmin_vo2max.
type Vo2maxRow struct {
	Date    string
	Vo2max  *float64
	RawJSON string
}

// UpsertVo2max upserts one garmin_vo2max row by date.
func (s *Store) UpsertVo2max(r Vo2maxRow) error {
	_, err := s.DB.Exec(`
		INSERT INTO garmin_vo2max (date, vo2max, raw_json)
		VALUES (?,?,?)
		ON CONFLICT(date) DO UPDATE SET
			vo2max=excluded.vo2max, raw_json=excluded.raw_json`,
		r.Date, r.Vo2max, r.RawJSON)
	return err
}

// Vo2maxPoint is one dated VO2max reading (vo2max may be nil when stored null).
type Vo2maxPoint struct {
	Date   string
	Vo2max *float64
}

// ListVo2max returns up to `limit` garmin_vo2max rows, most-recent-first by date.
func (s *Store) ListVo2max(limit int) ([]Vo2maxPoint, error) {
	rows, err := s.DB.Query(`
		SELECT date, vo2max FROM garmin_vo2max
		ORDER BY date DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Vo2maxPoint
	for rows.Next() {
		var p Vo2maxPoint
		if err := rows.Scan(&p.Date, &p.Vo2max); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// CountRecoveryDays returns the number of distinct calendar dates present
// across all four garmin_* tables.
func (s *Store) CountRecoveryDays() (int, error) {
	var n int
	err := s.DB.QueryRow(`
		SELECT COUNT(*) FROM (
			SELECT date FROM garmin_sleep
			UNION SELECT date FROM garmin_hrv
			UNION SELECT date FROM garmin_body_battery
			UNION SELECT date FROM garmin_rhr
		)`).Scan(&n)
	return n, err
}

// ListRecovery returns up to `days` merged recovery records, most-recent-first
// by date, full-outer-joining the four garmin_* tables on date.
func (s *Store) ListRecovery(days int) ([]RecoveryDay, error) {
	rows, err := s.DB.Query(`
		WITH dates AS (
			SELECT date FROM garmin_sleep
			UNION SELECT date FROM garmin_hrv
			UNION SELECT date FROM garmin_body_battery
			UNION SELECT date FROM garmin_rhr
		)
		SELECT d.date,
			s.duration_s, s.deep_s, s.light_s, s.rem_s, s.awake_s, s.score,
			h.last_night_avg_ms, h.status,
			b.charged, b.drained, b.high, b.low,
			r.resting_hr,
			(s.date IS NOT NULL) AS has_sleep,
			(h.date IS NOT NULL) AS has_hrv,
			(b.date IS NOT NULL) AS has_bb,
			(r.date IS NOT NULL) AS has_rhr
		FROM dates d
		LEFT JOIN garmin_sleep        s ON s.date = d.date
		LEFT JOIN garmin_hrv          h ON h.date = d.date
		LEFT JOIN garmin_body_battery b ON b.date = d.date
		LEFT JOIN garmin_rhr          r ON r.date = d.date
		ORDER BY d.date DESC
		LIMIT ?`, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RecoveryDay
	for rows.Next() {
		var (
			rd                                     RecoveryDay
			durationS, deepS, lightS, remS, awakeS *int64
			score, lastNightAvg, charged, drained  *int64
			high, low, restingHR                   *int64
			status                                 *string
			hasSleep, hasHrv, hasBB, hasRhr        bool
		)
		if err := rows.Scan(
			&rd.Date,
			&durationS, &deepS, &lightS, &remS, &awakeS, &score,
			&lastNightAvg, &status,
			&charged, &drained, &high, &low,
			&restingHR,
			&hasSleep, &hasHrv, &hasBB, &hasRhr,
		); err != nil {
			return nil, err
		}
		if hasSleep {
			rd.Sleep = &SleepFields{
				DurationS: durationS, DeepS: deepS, LightS: lightS,
				RemS: remS, AwakeS: awakeS, Score: score,
			}
		}
		if hasHrv {
			rd.HRV = &HrvFields{LastNightAvgMs: lastNightAvg, Status: status}
		}
		if hasBB {
			rd.BodyBattery = &BodyBatteryFields{Charged: charged, Drained: drained, High: high, Low: low}
		}
		if hasRhr {
			rd.RHR = &RhrFields{RestingHR: restingHR}
		}
		out = append(out, rd)
	}
	return out, rows.Err()
}
