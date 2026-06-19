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
