import type {
  Health,
  SourceStatus,
  Status,
  ConnectResponse,
  SyncSourceResult,
  SyncResponse,
  Activity,
  ActivitiesResponse,
  RecoveryDay,
  RecoveryResponse,
} from '../types';

describe('api types', () => {
  it('Status matches the /api/status contract', () => {
    const status: Status = {
      strava: {
        connected: true,
        athlete_id: 12345678,
        last_synced_at: '2026-06-19T05:00:30Z',
        last_run_at: '2026-06-19T05:00:30Z',
        status: 'ok',
        error: null,
      },
      garmin: {
        connected: true,
        last_synced_at: '2026-06-19T05:00:42Z',
        last_run_at: '2026-06-19T05:00:42Z',
        status: 'ok',
        error: null,
      },
      counts: { activities: 42, recovery_days: 30 },
    };
    expect(status.strava.athlete_id).toBe(12345678);
    expect(status.strava.status).toBe('ok');
    expect(status.counts.recovery_days).toBe(30);
  });

  it('SourceStatus supports null + never/error states', () => {
    const s: SourceStatus = {
      connected: false,
      last_synced_at: null,
      last_run_at: null,
      status: 'never',
      error: null,
    };
    const e: SourceStatus = {
      connected: false,
      last_synced_at: null,
      last_run_at: '2026-06-19T05:00:42Z',
      status: 'error',
      error: 'worker exit 1: re-run worker.py login',
    };
    expect(s.status).toBe('never');
    expect(e.error).toContain('re-run worker.py login');
  });

  it('ConnectResponse uses camelCase authorizeUrl', () => {
    const c: ConnectResponse = { authorizeUrl: 'https://www.strava.com/oauth/authorize?x=1' };
    expect(c.authorizeUrl).toContain('strava.com/oauth/authorize');
  });

  it('SyncResponse has per-source results', () => {
    const ok: SyncSourceResult = { status: 'ok', synced: 3, error: null };
    const sync: SyncResponse = {
      strava: ok,
      garmin: { status: 'error', synced: 0, error: 'worker exit 1: re-run worker.py login' },
    };
    expect(sync.strava.synced).toBe(3);
    expect(sync.garmin.status).toBe('error');
  });

  it('Activity allows null optional fields', () => {
    const a: Activity = {
      strava_id: 14820001234,
      name: 'Morning Run',
      type: 'Run',
      sport_type: null,
      start_time: '2026-06-18T06:12:00Z',
      start_time_local: null,
      distance_m: 10240.5,
      moving_time_s: 3120,
      elapsed_time_s: 3200,
      avg_hr: null,
      max_hr: null,
      avg_speed: null,
      max_speed: null,
      avg_cadence: null,
      elevation_gain_m: null,
    };
    const resp: ActivitiesResponse = { activities: [a] };
    expect(resp.activities[0].strava_id).toBe(14820001234);
  });

  it('RecoveryDay allows null sub-objects', () => {
    const day: RecoveryDay = {
      date: '2026-06-17',
      sleep: { duration_s: 25800, deep_s: 5400, light_s: 13800, rem_s: 4800, awake_s: 1800, score: 71 },
      hrv: null,
      body_battery: { charged: 58, drained: 80, high: 86, low: 12 },
      rhr: { resting_hr: 49 },
    };
    const resp: RecoveryResponse = { recovery: [day] };
    expect(resp.recovery[0].hrv).toBeNull();
    expect(resp.recovery[0].rhr?.resting_hr).toBe(49);
  });

  it('Health is a simple status object', () => {
    const h: Health = { status: 'ok' };
    expect(h.status).toBe('ok');
  });
});
