// Week-start helpers for the weekly plan. Wire format is a strict ISO date
// (YYYY-MM-DD); the backend rejects anything else with a 400.

const ISO_DATE_RE = /^\d{4}-\d{2}-\d{2}$/;

/**
 * The Monday (UTC) of the current ISO week, formatted as YYYY-MM-DD.
 * ISO weeks start on Monday, so Sunday rolls back to the previous Monday.
 */
export function currentMonday(now: Date = new Date()): string {
  const d = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate()));
  const dow = d.getUTCDay(); // 0=Sun..6=Sat
  const diff = (dow + 6) % 7; // days since Monday
  d.setUTCDate(d.getUTCDate() - diff);
  return d.toISOString().slice(0, 10);
}

/**
 * True when value is a real, well-formed YYYY-MM-DD calendar date.
 * Guards against shapes like 2026-13-40 that match the regex but aren't dates.
 */
export function isValidWeekStart(value: string): boolean {
  if (!ISO_DATE_RE.test(value)) return false;
  const ts = Date.parse(`${value}T00:00:00Z`);
  if (Number.isNaN(ts)) return false;
  // Reject overflow (e.g. 2026-02-31 normalizing to a different day).
  return new Date(ts).toISOString().slice(0, 10) === value;
}
