// SVG chart math — exact ports of the design files' spark()/chart() helpers
// (RunCoachPhone/CoachWeb logic scripts), so rendered curves match the mocks.

/** Polyline points for a sparkline over vals in a w×h viewBox. */
export function sparkPoints(vals: number[], w: number, h: number): string {
  if (vals.length < 2) return '';
  const mn = Math.min(...vals);
  const mx = Math.max(...vals);
  const r = mx - mn || 1;
  return vals
    .map(
      (v, i) =>
        ((i / (vals.length - 1)) * w).toFixed(1) + ',' + (h - ((v - mn) / r) * h).toFixed(1),
    )
    .join(' ');
}

/** Line polyline + area path (gradient fill) for the hero chart. */
export function areaChart(
  vals: number[],
  w: number,
  h: number,
  pad: number,
): { line: string; area: string } {
  if (vals.length < 2) return { line: '', area: '' };
  const mn = Math.min(...vals);
  const mx = Math.max(...vals);
  const r = mx - mn || 1;
  const pts = vals.map(
    (v, i) => [(i / (vals.length - 1)) * w, pad + (h - 2 * pad) * (1 - (v - mn) / r)] as const,
  );
  const line = pts.map((p) => p[0].toFixed(1) + ',' + p[1].toFixed(1)).join(' ');
  const area =
    'M' +
    pts[0][0].toFixed(1) +
    ',' +
    pts[0][1].toFixed(1) +
    ' ' +
    pts
      .slice(1)
      .map((p) => 'L' + p[0].toFixed(1) + ',' + p[1].toFixed(1))
      .join(' ') +
    ` L${w},${h} L0,${h} Z`;
  return { line, area };
}

/** Drop nulls from a progress series (weekly gaps are never interpolated). */
export function gapless(series: (number | null)[]): number[] {
  return series.filter((v): v is number => v !== null);
}

/** 331 sec/km -> "5:31". Non-positive -> "—". */
export function fmtPace(secPerKm: number): string {
  if (!Number.isFinite(secPerKm) || secPerKm <= 0) return '—';
  const total = Math.round(secPerKm);
  const m = Math.floor(total / 60);
  const s = total % 60;
  return `${m}:${String(s).padStart(2, '0')}`;
}


/** "48:20" / "1:02:10" from seconds. */
export function fmtDuration(totalS: number): string {
  const s = Math.max(0, Math.round(totalS));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const sec = s % 60;
  return h > 0
    ? `${h}:${String(m).padStart(2, '0')}:${String(sec).padStart(2, '0')}`
    : `${m}:${String(sec).padStart(2, '0')}`;
}
