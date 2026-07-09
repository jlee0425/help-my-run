// Signal-tile view models for the Today screen (design: 6 tiles, value +
// delta + 7-day sparkline). Pure functions — golden-tested.
import type { Activity, RecoveryDay } from '../api/types';
import { acRatio, runKmInWindow } from './activity';

export type TileColor = 'green' | 'amber' | 'red' | 'muted';

export type TileVM = {
  label: string;
  value: string;
  unit: string;
  delta: string;
  deltaColor: TileColor;
  spark: number[];
  sub: string;
};

function mean(vals: number[]): number | null {
  if (vals.length === 0) return null;
  return vals.reduce((a, b) => a + b, 0) / vals.length;
}

function series(recovery: RecoveryDay[], pick: (d: RecoveryDay) => number | null): number[] {
  return recovery
    .map(pick)
    .filter((v): v is number => v !== null && Number.isFinite(v));
}

function fmtSleep(durationS: number): string {
  const h = Math.floor(durationS / 3600);
  const m = Math.round((durationS % 3600) / 60);
  return `${h}:${String(m).padStart(2, '0')}`;
}

/**
 * Build the six signal tiles. `recovery` may arrive most-recent-first (the
 * API's order) — it is sorted ascending here. `vo2maxSeries` is the progress
 * report's vo2max weekly series (nullable gaps).
 */
export function buildTiles(
  recovery: RecoveryDay[],
  activities: Activity[],
  vo2maxSeries: (number | null)[],
  now: Date,
): TileVM[] {
  const days = [...recovery].sort((a, b) => (a.date < b.date ? -1 : 1));
  const tiles: TileVM[] = [];

  // HRV — last night vs 7-day mean.
  {
    const s = series(days, (d) => d.hrv?.last_night_avg_ms ?? null);
    const latest = s.at(-1) ?? null;
    const base = mean(s.slice(0, -1).slice(-7));
    let delta = '—';
    let color: TileColor = 'muted';
    if (latest !== null && base !== null) {
      const d = latest - base;
      delta = `${d >= 0 ? '+' : '−'}${Math.abs(Math.round(d))}`;
      color = d <= -base * 0.05 ? 'red' : d >= 0 ? 'green' : 'amber';
    }
    tiles.push({
      label: 'HRV',
      value: latest !== null ? String(Math.round(latest)) : '—',
      unit: 'ms',
      delta,
      deltaColor: color,
      spark: s.slice(-7),
      sub: base !== null ? `7-day ${Math.round(base)}` : 'no baseline',
    });
  }

  // SLEEP — duration vs 7-day mean; sub shows the score.
  {
    const s = series(days, (d) => d.sleep?.duration_s ?? null);
    const latest = s.at(-1) ?? null;
    const base = mean(s.slice(0, -1).slice(-7));
    const latestScore = [...days].reverse().find((d) => d.sleep?.score != null)?.sleep?.score;
    let delta = '—';
    let color: TileColor = 'muted';
    if (latest !== null && base !== null) {
      const dMin = Math.round((latest - base) / 60);
      const h = Math.floor(Math.abs(dMin) / 60);
      const m = Math.abs(dMin) % 60;
      delta = `${dMin >= 0 ? '+' : '−'}${h > 0 ? `${h}:${String(m).padStart(2, '0')}` : `${m}m`}`;
      color = dMin <= -45 ? 'red' : dMin >= 0 ? 'green' : 'amber';
    }
    tiles.push({
      label: 'SLEEP',
      value: latest !== null ? fmtSleep(latest) : '—',
      unit: 'h',
      delta,
      deltaColor: color,
      spark: s.slice(-7).map((v) => v / 3600),
      sub: latestScore != null ? `score ${latestScore}` : ' ',
    });
  }

  // BODY BATTERY — today's high.
  {
    const s = series(days, (d) => d.body_battery?.high ?? null);
    const latest = s.at(-1) ?? null;
    tiles.push({
      label: 'BODY BATTERY',
      value: latest !== null ? String(Math.round(latest)) : '—',
      unit: '',
      delta: latest !== null && latest < 50 ? 'low' : latest !== null ? 'ok' : '—',
      deltaColor: latest === null ? 'muted' : latest < 50 ? 'amber' : 'green',
      spark: s.slice(-7),
      sub: 'peak today',
    });
  }

  // RESTING HR — last night vs 7-day mean (lower is better).
  {
    const s = series(days, (d) => d.rhr?.resting_hr ?? null);
    const latest = s.at(-1) ?? null;
    const base = mean(s.slice(0, -1).slice(-7));
    let delta = '—';
    let color: TileColor = 'muted';
    if (latest !== null && base !== null) {
      const d = latest - base;
      delta = `${d >= 0 ? '+' : '−'}${Math.abs(Math.round(d))}`;
      color = d >= 3 ? 'red' : d <= 0 ? 'green' : 'amber';
    }
    tiles.push({
      label: 'RESTING HR',
      value: latest !== null ? String(Math.round(latest)) : '—',
      unit: 'bpm',
      delta,
      deltaColor: color,
      spark: s.slice(-7),
      sub: base !== null ? `base ${Math.round(base)}` : 'no baseline',
    });
  }

  // LOAD a:c — 7d run km vs 28d weekly average.
  {
    const ratio = acRatio(activities, now);
    const acute = runKmInWindow(activities, 7, now);
    const chronicWeekly = runKmInWindow(activities, 28, now) / 4;
    tiles.push({
      label: 'LOAD a:c',
      value: ratio !== null ? ratio.toFixed(2) : '—',
      unit: '',
      delta: ratio === null ? '—' : ratio >= 0.8 && ratio <= 1.3 ? 'ok' : ratio > 1.5 ? 'high' : 'off',
      deltaColor:
        ratio === null ? 'muted' : ratio >= 0.8 && ratio <= 1.3 ? 'green' : ratio > 1.5 ? 'red' : 'amber',
      spark: [],
      sub: ratio !== null ? `${acute.toFixed(0)} / ${chronicWeekly.toFixed(0)} km` : 'no run history',
    });
  }

  // VO2MAX — latest reading from the progress series.
  {
    const vals = vo2maxSeries.filter((v): v is number => v !== null);
    const latest = vals.at(-1) ?? null;
    const prev = vals.at(-2) ?? null;
    let delta = '—';
    if (latest !== null && prev !== null) {
      const d = Math.round((latest - prev) * 10) / 10;
      delta = d === 0 ? '·' : `${d > 0 ? '+' : '−'}${Math.abs(d)}`;
    }
    tiles.push({
      label: 'VO2MAX',
      value: latest !== null ? String(latest) : '—',
      unit: '',
      delta,
      deltaColor: 'muted',
      spark: vals.slice(-7),
      sub: 'Garmin',
    });
  }

  return tiles;
}
