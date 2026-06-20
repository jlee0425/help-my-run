export interface Health { status: string; }

export interface SourceStatus {
  connected: boolean;
  last_synced_at: string | null;
  last_run_at: string | null;
  status: 'ok' | 'error' | 'never';
  error: string | null;
}
export interface Status {
  strava: SourceStatus & { athlete_id: number }; // M0 always emits a non-null athlete_id: a strava_tokens row exists only after a successful OAuth that includes an athlete.
  garmin: SourceStatus;
  counts: { activities: number; recovery_days: number };
}

export interface ConnectResponse { authorizeUrl: string; }

export interface SyncSourceResult { status: 'ok' | 'error'; synced: number; error: string | null; }
export interface SyncResponse { strava: SyncSourceResult; garmin: SyncSourceResult; }

export interface Activity {
  strava_id: number;
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
}
export interface ActivitiesResponse { activities: Activity[]; }

export interface RecoveryDay {
  date: string;
  sleep: { duration_s: number | null; deep_s: number | null; light_s: number | null; rem_s: number | null; awake_s: number | null; score: number | null } | null;
  hrv: { last_night_avg_ms: number | null; status: string | null } | null;
  body_battery: { charged: number | null; drained: number | null; high: number | null; low: number | null } | null;
  rhr: { resting_hr: number | null } | null;
}
export interface RecoveryResponse { recovery: RecoveryDay[]; }

// --- M1 types (snake_case wire JSON) ---

export type Load = 'low' | 'med' | 'high';

export interface CrossFitDay {
  date: string; dow: string; has_crossfit: boolean;
  focus: string; cns_load: Load; leg_load: Load; notes: string;
}
export interface CrossFitWeek { week_start: string; days: CrossFitDay[]; }

export interface PlanDay {
  date: string; dow: string; run_type: string; distance_km: number;
  pace_target: string; time_note: string; optional_if_cns: boolean; rationale: string;
}
export interface Plan {
  id?: number; week_start: string; generated_at?: string;
  fitness_summary: string; weekly_target_km: number;
  days: PlanDay[]; week_rationale: string; one_flag: string;
}

export interface AthleteProfile {
  target_weekly_km: number;
  progression_mode: 'build' | 'hold';
  zone2_ceiling_bpm: number | null;
  threshold_bpm: number | null;
  max_hr_bpm: number | null;
  run_constraints_json: string;
  goal_text: string;
  updated_at?: string;
}

export interface Fitness {
  weekly_volume_km: number;
  four_week_avg_km: number;
  acute_chronic_ratio: number;
  easy_pace: string;
  threshold_pace: string;
  recovery_trend: string;
  safe_weekly_target_km: number;
  is_cutback_week: boolean;
}
