// Wire types — mirror backend/internal/api/dto.go exactly (snake_case).

export type Status = {
  garmin: {
    connected: boolean;
    last_synced_at: string | null;
    last_run_at: string | null;
    status: string;
    error: string | null;
  };
  counts: { activities: number; recovery_days: number };
  agent_next_run: string | null;
  agent_enabled: boolean;
};

export type Activity = {
  activity_id: number;
  name: string;
  type: string;
  sport_type: string | null;
  start_time: string;
  start_time_local: string | null;
  distance_m: number;
  moving_time_s: number;
  elapsed_time_s: number;
  avg_hr: number | null;
  max_hr: number | null;
  avg_speed: number | null;
  max_speed: number | null;
  avg_cadence: number | null;
  elevation_gain_m: number | null;
};

export type RecoveryDay = {
  date: string;
  sleep: {
    duration_s: number | null;
    deep_s: number | null;
    light_s: number | null;
    rem_s: number | null;
    awake_s: number | null;
    score: number | null;
  } | null;
  hrv: { last_night_avg_ms: number | null; status: string | null } | null;
  body_battery: {
    charged: number | null;
    drained: number | null;
    high: number | null;
    low: number | null;
  } | null;
  rhr: { resting_hr: number | null } | null;
};

export type PlanDay = {
  date: string;
  dow: string;
  run_type: string;
  distance_km: number;
  pace_target: string;
  time_note: string;
  optional_if_cns: boolean;
  rationale: string;
};

export type Plan = {
  id: number;
  week_start: string;
  generated_at: string;
  fitness_summary: string;
  weekly_target_km: number;
  days: PlanDay[];
  week_rationale: string;
  one_flag: string;
};

export type ReadinessDrivers = {
  date: string;
  sleep_hours: number | null;
  sleep_score: number | null;
  hrv_last_night_ms: number | null;
  hrv_baseline_ms: number | null;
  hrv_delta_pct: number | null;
  rhr_last_night: number | null;
  rhr_baseline: number | null;
  rhr_delta_bpm: number | null;
  body_battery_high: number | null;
  recovery_trend: string;
  data_complete: boolean;
};

export type Today = {
  date: string;
  readiness_color: 'green' | 'amber' | 'red' | string;
  drivers: ReadinessDrivers;
  reasons: string[];
  action: 'STAND' | 'SOFTEN' | 'MOVE' | 'REST_DAY' | string;
  original_session: PlanDay | null;
  effective_session: PlanDay | null;
  rationale: string;
  source: string;
  stale: boolean;
};

export type TrendSignal = {
  key: string;
  label: string;
  unit: string;
  current: number | null;
  baseline: number | null;
  delta_abs: number | null;
  direction: 'up' | 'down' | 'flat';
  lower_is_better: boolean;
  series: (number | null)[];
};

export type ProgressReport = {
  weeks: number;
  generated_at: string;
  signals: TrendSignal[];
  enough_data: boolean;
};

export type ZoneTime = { zone: number; seconds: number; pct: number };

export type StreamAnalysis = {
  activity_id: number;
  has_stream: boolean;
  has_hr: boolean;
  time_in_zone: ZoneTime[];
  decoupling_pct: number | null;
  pa_hr_first: number | null;
  pa_hr_second: number | null;
  zones: { z1_hi: number; z2_hi: number; z3_hi: number; z4_hi: number };
  source: string;
  computed_at: string;
};

export type Profile = {
  target_weekly_km: number;
  progression_mode: string;
  zone2_ceiling_bpm: number | null;
  threshold_bpm: number | null;
  max_hr_bpm: number | null;
  run_constraints_json: string;
  goal_text: string;
  daily_run_time: string;
  timezone: string;
  agent_enabled: boolean;
  goals_json: string;
  week_json: string;
  guardrails_json: string;
  updated_at?: string;
};

export type ChatMessage = {
  id: number;
  role: 'user' | 'assistant' | string;
  content: string;
  created_at: string;
};

export type AgentRunResult = {
  date: string;
  skipped: boolean;
  readiness_color: string;
  action: string;
  source: string;
  stale: boolean;
  pushed: boolean;
  error: string | null;
};
