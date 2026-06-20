import { currentMonday, isValidWeekStart } from '../week';

describe('currentMonday', () => {
  it('returns the Monday of the same ISO week for a mid-week date', () => {
    // 2026-06-20 is a Saturday → Monday is 2026-06-15.
    expect(currentMonday(new Date('2026-06-20T10:00:00Z'))).toBe('2026-06-15');
  });

  it('returns the same date when given a Monday', () => {
    expect(currentMonday(new Date('2026-06-15T00:00:00Z'))).toBe('2026-06-15');
  });

  it('rolls Sunday back to the previous Monday (ISO weeks start Monday)', () => {
    // 2026-06-21 is a Sunday → Monday is 2026-06-15.
    expect(currentMonday(new Date('2026-06-21T23:59:00Z'))).toBe('2026-06-15');
  });

  it('always returns a YYYY-MM-DD string that is a Monday', () => {
    const out = currentMonday(new Date('2026-12-31T12:00:00Z'));
    expect(out).toMatch(/^\d{4}-\d{2}-\d{2}$/);
    expect(new Date(`${out}T00:00:00Z`).getUTCDay()).toBe(1);
  });
});

describe('isValidWeekStart', () => {
  it('accepts a well-formed calendar date', () => {
    expect(isValidWeekStart('2026-06-15')).toBe(true);
  });

  it('rejects wrong shapes', () => {
    expect(isValidWeekStart('')).toBe(false);
    expect(isValidWeekStart('2026-6-15')).toBe(false);
    expect(isValidWeekStart('06/15/2026')).toBe(false);
    expect(isValidWeekStart('not-a-date')).toBe(false);
    expect(isValidWeekStart('2026-06-15T00:00:00Z')).toBe(false);
  });

  it('rejects out-of-range / overflow dates that match the regex', () => {
    expect(isValidWeekStart('2026-13-01')).toBe(false);
    expect(isValidWeekStart('2026-02-31')).toBe(false);
    expect(isValidWeekStart('2026-00-10')).toBe(false);
  });
});
