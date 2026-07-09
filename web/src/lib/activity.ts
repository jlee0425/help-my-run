import type { Activity } from '../api/types';

/** Canonical run typeKeys (mirrors backend metrics.IsRun vocabulary). */
export function isRunType(t: string): boolean {
  return [
    'run',
    'running',
    'trail_running',
    'treadmill_running',
    'track_running',
    'street_running',
    'virtual_run',
  ].includes(t.toLowerCase());
}

/** Sum of run km for activities within (now - days, now]. */
export function runKmInWindow(activities: Activity[], days: number, now: Date): number {
  const start = now.getTime() - days * 86400_000;
  let km = 0;
  for (const a of activities) {
    if (!isRunType(a.type)) continue;
    const t = Date.parse(a.start_time);
    if (Number.isNaN(t) || t <= start || t > now.getTime()) continue;
    km += a.distance_m / 1000;
  }
  return km;
}

/** Acute(7d) : chronic(28d weekly avg) run-load ratio; null without history. */
export function acRatio(activities: Activity[], now: Date): number | null {
  const acute = runKmInWindow(activities, 7, now);
  const chronicWeekly = runKmInWindow(activities, 28, now) / 4;
  if (chronicWeekly <= 0) return null;
  return acute / chronicWeekly;
}
