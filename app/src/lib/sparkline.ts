// 7 block levels. Index 0 = lowest, 6 = highest. Full 7-level ramp ▁▂▃▄▅▆▇
// (superset of the spec §8 6-glyph example; finer resolution).
const BLOCKS = ['▁', '▂', '▃', '▄', '▅', '▆', '▇'];
const GAP = ' ';

/**
 * Render a numeric series as a unicode-block sparkline.
 * null/undefined/NaN entries become a blank (gap), never a fabricated point.
 * Output string length === series.length. A finite series with no spread
 * renders all mid-level blocks.
 */
export function sparkline(series: (number | null | undefined)[]): string {
  const finite = series.filter(
    (v): v is number => typeof v === 'number' && Number.isFinite(v),
  );
  if (finite.length === 0) return series.map(() => GAP).join('');

  const min = Math.min(...finite);
  const max = Math.max(...finite);
  const span = max - min;
  const last = BLOCKS.length - 1;

  return series
    .map((v) => {
      if (typeof v !== 'number' || !Number.isFinite(v)) return GAP;
      if (span === 0) return BLOCKS[Math.floor(last / 2)]; // flat series -> mid
      const idx = Math.round(((v - min) / span) * last);
      return BLOCKS[idx];
    })
    .join('');
}
