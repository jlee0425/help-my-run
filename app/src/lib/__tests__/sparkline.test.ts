import { sparkline } from '../sparkline';

describe('sparkline', () => {
  it('scales a strictly increasing series across the block ramp (min/mid/max)', () => {
    // 7-level ramp ▁▂▃▄▅▆▇; idx = round((v-min)/(max-min) * 6)
    // [1,2,3]: 1->0 (▁ ▁), 2->3 (▄ ▄), 3->6 (▇ ▇)
    expect(sparkline([1, 2, 3])).toBe('▁▄▇');
  });

  it('renders null gaps as a blank space and a flat series as mid-level blocks', () => {
    // [5,null,5]: finite values are flat (span 0) -> mid block ▄ (▄); null -> ' '
    expect(sparkline([5, null, 5])).toBe('▄ ▄');
  });

  it('returns an empty string for an empty series', () => {
    expect(sparkline([])).toBe('');
  });

  it('renders an all-null series as all blanks (length preserved)', () => {
    expect(sparkline([null, null])).toBe('  ');
  });

  it('treats undefined and NaN as gaps', () => {
    expect(sparkline([undefined, NaN, 5])).toBe('  ▄');
  });

  it('always returns a string whose length equals the series length', () => {
    const series = [350, null, 345, 342, null, 340, 338];
    expect(sparkline(series)).toHaveLength(series.length);
  });
});
