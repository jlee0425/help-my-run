// Trends-screen view models — pure functions over the progress report,
// activities, and recovery days. Golden-tested.
import type { Activity, ProgressReport, RecoveryDay, TrendSignal } from '../api/types';
import { isRunType } from './activity';
import { fmtPace, gapless } from '../ui/charts';

function signal(report: ProgressReport, key: string): TrendSignal | undefined {
  return report.signals.find((s) => s.key === key);
}

export function paceSeries(
  report: ProgressReport,
): { values: number[]; now: string; then: string; deltaLabel: string } | null {
  const s = signal(report, 'pace_at_hr');
  if (!s) return null;
  const values = gapless(s.series);
  if (values.length < 2) return null;
  const delta = s.delta_abs ?? values[values.length - 1] - values[0];
  const arrow = delta <= 0 ? '▼' : '▲';
  return {
    values,
    now: fmtPace(values[values.length - 1]),
    then: fmtPace(values[0]),
    deltaLabel: `${arrow} ${Math.abs(Math.round(delta))}s/km`,
  };
}

/** Direction+polarity → good(green)/bad(red)/flat(muted) chip color. */
function chipColor(s: TrendSignal): string {
  if (s.direction === 'flat') return 'var(--muted)';
  const improved = s.lower_is_better ? s.direction === 'down' : s.direction === 'up';
  return improved ? 'var(--green)' : 'var(--red)';
}

export function deltaChips(
  report: ProgressReport,
): { label: string; value: string; color: string }[] {
  const defs: { key: string; label: string; fmt: (d: number) => string }[] = [
    { key: 'pace_at_hr', label: 'EASY PACE', fmt: (d) => `${d <= 0 ? '▼' : '▲'}${Math.abs(Math.round(d))}s` },
    { key: 'hrv_baseline', label: 'HRV', fmt: (d) => `${d >= 0 ? '▲' : '▼'}${Math.abs(Math.round(d))}` },
    { key: 'resting_hr', label: 'RESTING HR', fmt: (d) => `${d >= 0 ? '▲' : '▼'}${Math.abs(Math.round(d))}` },
  ];
  const out: { label: string; value: string; color: string }[] = [];
  for (const def of defs) {
    const s = signal(report, def.key);
    if (!s || s.delta_abs == null) continue;
    out.push({ label: def.label, value: def.fmt(s.delta_abs), color: chipColor(s) });
  }
  return out;
}

export function miniSeries(
  report: ProgressReport,
  key: 'hrv_baseline' | 'resting_hr',
): { values: number[]; current: string; delta: string; good: boolean } | null {
  const s = signal(report, key);
  if (!s) return null;
  const values = gapless(s.series);
  if (values.length < 2 || s.current == null) return null;
  const d = s.delta_abs ?? 0;
  const improved = s.lower_is_better ? d <= 0 : d >= 0;
  return {
    values,
    current: String(Math.round(s.current)),
    delta: `${d >= 0 ? '▲' : '▼'}${Math.abs(Math.round(d))}`,
    good: improved,
  };
}

/** Per-ISO-week run vs non-run moving minutes over the last `weeks` weeks. */
export function loadSplit(
  activities: Activity[],
  weeks: number,
  now: Date,
): { label: string; run: number; cf: number }[] {
  const out: { label: string; run: number; cf: number }[] = [];
  for (let i = weeks - 1; i >= 0; i--) {
    const end = now.getTime() - i * 7 * 86400_000;
    const start = end - 7 * 86400_000;
    let run = 0;
    let cf = 0;
    for (const a of activities) {
      const t = Date.parse(a.start_time);
      if (Number.isNaN(t) || t <= start || t > end) continue;
      const minutes = a.moving_time_s / 60;
      if (isRunType(a.type)) run += minutes;
      else cf += minutes;
    }
    out.push({ label: `w${weeks - i}`, run: Math.round(run), cf: Math.round(cf) });
  }
  return out;
}

/** Overall run share of total load minutes ("Running ~55%"). */
export function runSharePct(split: { run: number; cf: number }[]): number | null {
  const run = split.reduce((a, w) => a + w.run, 0);
  const total = run + split.reduce((a, w) => a + w.cf, 0);
  if (total <= 0) return null;
  return Math.round((run / total) * 100);
}

/**
 * SLEEP → PACE LINK: compare next-day easy-run pace after short (<6h) vs
 * normal (≥7h) nights, at comparable heart rate. Requires ≥3 runs per bucket,
 * else null (the card hides).
 */
export function sleepPaceLink(
  recovery: RecoveryDay[],
  activities: Activity[],
): { slowerSecPerKm: number; nights: number } | null {
  const sleepByDate = new Map<string, number>();
  for (const d of recovery) {
    if (d.sleep?.duration_s != null) sleepByDate.set(d.date, d.sleep.duration_s / 3600);
  }
  const runs = activities.filter(
    (a) => isRunType(a.type) && a.distance_m > 0 && a.moving_time_s > 0 && a.avg_hr != null,
  );
  if (runs.length < 6) return null;
  const hrs = runs.map((a) => a.avg_hr as number).sort((a, b) => a - b);
  const medianHR = hrs[Math.floor(hrs.length / 2)];

  const short: number[] = [];
  const normal: number[] = [];
  for (const a of runs) {
    if (Math.abs((a.avg_hr as number) - medianHR) > 8) continue; // comparable effort only
    const date = a.start_time.slice(0, 10);
    const sleepH = sleepByDate.get(date);
    if (sleepH == null) continue;
    const pace = a.moving_time_s / (a.distance_m / 1000);
    if (sleepH < 6) short.push(pace);
    else if (sleepH >= 7) normal.push(pace);
  }
  if (short.length < 3 || normal.length < 3) return null;
  const mean = (v: number[]) => v.reduce((a, b) => a + b, 0) / v.length;
  const slower = Math.round(mean(short) - mean(normal));
  if (slower <= 0) return null; // only surface the insight when it exists
  return { slowerSecPerKm: slower, nights: short.length };
}
