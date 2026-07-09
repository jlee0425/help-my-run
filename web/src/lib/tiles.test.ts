import { describe, expect, it } from 'vitest';
import type { Activity, RecoveryDay } from '../api/types';
import { buildTiles } from './tiles';
import { acRatio, isRunType, runKmInWindow } from './activity';

const NOW = new Date('2026-07-09T08:00:00Z');

function day(date: string, hrv: number | null, rhr: number | null, sleepS: number | null, bbHigh: number | null): RecoveryDay {
  return {
    date,
    sleep: sleepS !== null ? { duration_s: sleepS, deep_s: null, light_s: null, rem_s: null, awake_s: null, score: 64 } : null,
    hrv: hrv !== null ? { last_night_avg_ms: hrv, status: null } : null,
    body_battery: bbHigh !== null ? { charged: null, drained: null, high: bbHigh, low: null } : null,
    rhr: rhr !== null ? { resting_hr: rhr } : null,
  };
}

function run(startISO: string, km: number, type = 'running'): Activity {
  return {
    activity_id: Math.round(Math.random() * 1e9),
    name: 'run',
    type,
    sport_type: null,
    start_time: startISO,
    start_time_local: null,
    distance_m: km * 1000,
    moving_time_s: km * 360,
    elapsed_time_s: km * 360,
    avg_hr: 145,
    max_hr: 160,
    avg_speed: 2.8,
    max_speed: 3.5,
    avg_cadence: null,
    elevation_gain_m: null,
  };
}

// API order: most-recent-first (buildTiles must sort ascending itself).
const RECOVERY: RecoveryDay[] = [
  day('2026-07-09', 48, 54, 6 * 3600 + 120, 41),
  day('2026-07-08', 52, 53, 6.6 * 3600, 45),
  day('2026-07-07', 55, 52, 6.2 * 3600, 50),
  day('2026-07-06', 57, 50, 6.9 * 3600, 55),
  day('2026-07-05', 60, 51, 7.4 * 3600, 60),
  day('2026-07-04', 62, 49, 6.6 * 3600, 72),
  day('2026-07-03', 58, 50, 7.2 * 3600, 78),
];

describe('buildTiles', () => {
  const tiles = buildTiles(RECOVERY, [], [null, 50.5, 51.0, 52.0], NOW);
  const byLabel = Object.fromEntries(tiles.map((t) => [t.label, t]));

  it('produces the six design tiles in order', () => {
    expect(tiles.map((t) => t.label)).toEqual([
      'HRV',
      'SLEEP',
      'BODY BATTERY',
      'RESTING HR',
      'LOAD a:c',
      'VO2MAX',
    ]);
  });

  it('HRV: latest vs prior-7-day mean, red when ≥5% below', () => {
    // prior mean = (52+55+57+60+62+58)/6 = 57.33; latest 48 → −9 red
    expect(byLabel['HRV'].value).toBe('48');
    expect(byLabel['HRV'].delta).toBe('−9');
    expect(byLabel['HRV'].deltaColor).toBe('red');
    expect(byLabel['HRV'].sub).toBe('7-day 57');
    expect(byLabel['HRV'].spark).toHaveLength(7);
    expect(byLabel['HRV'].spark.at(-1)).toBe(48);
  });

  it('RESTING HR: +bpm over base is red at ≥3', () => {
    // prior mean = (53+52+50+51+49+50)/6 = 50.83; latest 54 → +3 red
    expect(byLabel['RESTING HR'].value).toBe('54');
    expect(byLabel['RESTING HR'].delta).toBe('+3');
    expect(byLabel['RESTING HR'].deltaColor).toBe('red');
  });

  it('SLEEP: h:mm value and score sub', () => {
    expect(byLabel['SLEEP'].value).toBe('6:02');
    expect(byLabel['SLEEP'].sub).toBe('score 64');
    expect(byLabel['SLEEP'].deltaColor).toBe('red'); // ~−48m vs mean
  });

  it('BODY BATTERY: amber when peak < 50', () => {
    expect(byLabel['BODY BATTERY'].value).toBe('41');
    expect(byLabel['BODY BATTERY'].deltaColor).toBe('amber');
  });

  it('VO2MAX: latest + delta vs previous, Garmin sub', () => {
    expect(byLabel['VO2MAX'].value).toBe('52');
    expect(byLabel['VO2MAX'].delta).toBe('+1');
    expect(byLabel['VO2MAX'].sub).toBe('Garmin');
  });

  it('LOAD a:c: ratio of 7d km to 28d weekly average, green in band', () => {
    const acts = [
      run('2026-07-08T06:00:00Z', 10),
      run('2026-07-05T06:00:00Z', 8),
      run('2026-06-28T06:00:00Z', 9),
      run('2026-06-20T06:00:00Z', 9),
      run('2026-06-14T06:00:00Z', 9),
      run('2026-07-07T06:00:00Z', 5, 'strength_training'), // ignored
    ];
    const t = buildTiles(RECOVERY, acts, [], NOW).find((x) => x.label === 'LOAD a:c')!;
    // acute = 18; chronic weekly = (18+9+9)/4 = 9  → hmm 28d window from 6/11: runs 7/8,7/5,6/28,6/20,6/14 = 45/4 = 11.25 → 1.6
    expect(t.value).toBe((18 / (45 / 4)).toFixed(2));
    expect(t.deltaColor).toBe('red'); // 1.6 > 1.5
    expect(t.sub).toContain('18 / 11 km');
  });

  it('handles empty inputs without crashing', () => {
    const t = buildTiles([], [], [], NOW);
    expect(t).toHaveLength(6);
    expect(t.every((x) => x.value === '—' || x.value.length > 0)).toBe(true);
  });
});

describe('activity helpers', () => {
  it('isRunType covers Garmin run vocabulary, rejects others', () => {
    expect(isRunType('running')).toBe(true);
    expect(isRunType('trail_running')).toBe(true);
    expect(isRunType('Strength_Training'.toLowerCase())).toBe(false);
    expect(isRunType('indoor_cycling')).toBe(false);
  });

  it('runKmInWindow respects the window edges', () => {
    const acts = [run('2026-07-08T06:00:00Z', 10), run('2026-06-30T06:00:00Z', 7)];
    expect(runKmInWindow(acts, 7, NOW)).toBe(10);
    expect(runKmInWindow(acts, 28, NOW)).toBe(17);
  });

  it('acRatio null without chronic history', () => {
    expect(acRatio([], NOW)).toBeNull();
  });
});
