import { describe, expect, it } from 'vitest';
import type { Activity, ProgressReport, RecoveryDay, TrendSignal } from '../api/types';
import { deltaChips, loadSplit, miniSeries, paceSeries, runSharePct, sleepPaceLink } from './trends';

const NOW = new Date('2026-07-09T12:00:00Z');

function sig(key: string, series: (number | null)[], lowerIsBetter: boolean): TrendSignal {
  const vals = series.filter((v): v is number => v !== null);
  const current = vals.at(-1) ?? null;
  const baseline = vals[0] ?? null;
  const delta = current !== null && baseline !== null ? current - baseline : null;
  return {
    key,
    label: key,
    unit: '',
    current,
    baseline,
    delta_abs: delta,
    direction: delta === null || delta === 0 ? 'flat' : delta > 0 ? 'up' : 'down',
    lower_is_better: lowerIsBetter,
    series,
  };
}

const REPORT: ProgressReport = {
  weeks: 12,
  generated_at: '2026-07-09T00:00:00Z',
  enough_data: true,
  signals: [
    sig('pace_at_hr', [352, null, 347, 343, null, 338, 331], true),
    sig('hrv_baseline', [55, 56, 58, 60, 61], false),
    sig('resting_hr', [53, 52, 51, 50, 49], true),
    sig('vo2max', [50, 51], false),
  ],
};

function run(startISO: string, km: number, avgHR: number, paceSecPerKm: number, type = 'running'): Activity {
  return {
    activity_id: Math.round(Math.random() * 1e9),
    name: 'x',
    type,
    sport_type: null,
    start_time: startISO,
    start_time_local: null,
    distance_m: km * 1000,
    moving_time_s: km * paceSecPerKm,
    elapsed_time_s: km * paceSecPerKm,
    avg_hr: avgHR,
    max_hr: null,
    avg_speed: 1000 / paceSecPerKm,
    max_speed: null,
    avg_cadence: null,
    elevation_gain_m: null,
  };
}

function sleepDay(date: string, hours: number): RecoveryDay {
  return {
    date,
    sleep: { duration_s: hours * 3600, deep_s: null, light_s: null, rem_s: null, awake_s: null, score: null },
    hrv: null,
    body_battery: null,
    rhr: null,
  };
}

describe('paceSeries', () => {
  it('drops gaps and formats endpoints + delta', () => {
    const p = paceSeries(REPORT)!;
    expect(p.values).toEqual([352, 347, 343, 338, 331]);
    expect(p.then).toBe('5:52');
    expect(p.now).toBe('5:31');
    expect(p.deltaLabel).toBe('▼ 21s/km');
  });

  it('null when under 2 points', () => {
    const r: ProgressReport = { ...REPORT, signals: [sig('pace_at_hr', [null, 331], true)] };
    expect(paceSeries(r)).toBeNull();
  });
});

describe('deltaChips', () => {
  it('colors improvements green regardless of direction polarity', () => {
    const chips = deltaChips(REPORT);
    expect(chips).toEqual([
      { label: 'EASY PACE', value: '▼21s', color: 'var(--green)' }, // down + lower_is_better
      { label: 'HRV', value: '▲6', color: 'var(--green)' }, // up + higher_is_better
      { label: 'RESTING HR', value: '▼4', color: 'var(--green)' }, // down + lower_is_better
    ]);
  });

  it('colors regressions red', () => {
    const r: ProgressReport = { ...REPORT, signals: [sig('resting_hr', [49, 54], true)] };
    expect(deltaChips(r)).toEqual([{ label: 'RESTING HR', value: '▲5', color: 'var(--red)' }]);
  });
});

describe('miniSeries', () => {
  it('summarizes hrv + rhr minis with polarity-aware goodness', () => {
    const hrv = miniSeries(REPORT, 'hrv_baseline')!;
    expect(hrv.current).toBe('61');
    expect(hrv.delta).toBe('▲6');
    expect(hrv.good).toBe(true);
    const rhr = miniSeries(REPORT, 'resting_hr')!;
    expect(rhr.current).toBe('49');
    expect(rhr.delta).toBe('▼4');
    expect(rhr.good).toBe(true);
  });
});

describe('loadSplit + runSharePct', () => {
  it('buckets run vs non-run minutes per week, oldest first', () => {
    const acts = [
      run('2026-07-08T06:00:00Z', 10, 145, 330), // this week: 55 run min
      run('2026-07-07T06:00:00Z', 0, 0, 0, 'strength_training'), // 0 min, ignored value-wise
      { ...run('2026-07-06T06:00:00Z', 1, 150, 60, 'indoor_cardio'), moving_time_s: 3600 }, // 60 cf min
      run('2026-06-30T06:00:00Z', 8, 145, 330), // last week: 44 run min
    ];
    const split = loadSplit(acts, 2, NOW);
    expect(split).toHaveLength(2);
    expect(split[0].label).toBe('w1');
    expect(split[0].run).toBe(44);
    expect(split[1].run).toBe(55);
    expect(split[1].cf).toBe(60);
    expect(runSharePct(split)).toBe(Math.round(((44 + 55) / (44 + 55 + 60)) * 100));
  });

  it('runSharePct null with no load', () => {
    expect(runSharePct([{ run: 0, cf: 0 }])).toBeNull();
  });
});

describe('sleepPaceLink', () => {
  it('finds slower next-day pace after short nights at comparable HR', () => {
    const recovery = [
      sleepDay('2026-07-01', 5.5),
      sleepDay('2026-07-02', 5.4),
      sleepDay('2026-07-03', 5.0),
      sleepDay('2026-07-04', 7.5),
      sleepDay('2026-07-05', 7.4),
      sleepDay('2026-07-06', 8.0),
    ];
    const acts = [
      run('2026-07-01T07:00:00Z', 8, 145, 341),
      run('2026-07-02T07:00:00Z', 8, 146, 342),
      run('2026-07-03T07:00:00Z', 8, 144, 340),
      run('2026-07-04T07:00:00Z', 8, 145, 330),
      run('2026-07-05T07:00:00Z', 8, 146, 331),
      run('2026-07-06T07:00:00Z', 8, 145, 329),
    ];
    const link = sleepPaceLink(recovery, acts)!;
    expect(link).not.toBeNull();
    expect(link.slowerSecPerKm).toBe(11); // (341+342+340)/3 − (330+331+329)/3
    expect(link.nights).toBe(3);
  });

  it('null when a bucket has under 3 comparable runs', () => {
    const recovery = [sleepDay('2026-07-01', 5.5), sleepDay('2026-07-04', 7.5)];
    const acts = [
      run('2026-07-01T07:00:00Z', 8, 145, 341),
      run('2026-07-04T07:00:00Z', 8, 145, 330),
      run('2026-07-05T07:00:00Z', 8, 145, 331),
      run('2026-07-06T07:00:00Z', 8, 145, 329),
      run('2026-07-07T07:00:00Z', 8, 145, 328),
      run('2026-07-08T07:00:00Z', 8, 145, 327),
    ];
    expect(sleepPaceLink(recovery, acts)).toBeNull();
  });
});
