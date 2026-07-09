import { describe, expect, it } from 'vitest';
import { areaChart, fmtDuration, fmtPace, gapless, sparkPoints } from './charts';

describe('chart math (design-file ports)', () => {
  it('sparkPoints maps min->bottom, max->top across the width', () => {
    expect(sparkPoints([1, 2], 110, 26)).toBe('0.0,26.0 110.0,0.0');
    // flat series: r falls back to 1, all points at the bottom
    expect(sparkPoints([5, 5, 5], 100, 20)).toBe('0.0,20.0 50.0,20.0 100.0,20.0');
    expect(sparkPoints([1], 100, 20)).toBe('');
  });

  it('areaChart produces a closed area path ending at the baseline', () => {
    const { line, area } = areaChart([10, 20], 100, 50, 5);
    expect(line).toBe('0.0,45.0 100.0,5.0');
    expect(area).toBe('M0.0,45.0 L100.0,5.0 L100,50 L0,50 Z');
  });

  it('gapless drops nulls only', () => {
    expect(gapless([null, 1, null, 2, 0, null])).toEqual([1, 2, 0]);
  });

  it('fmtPace renders M:SS', () => {
    expect(fmtPace(331)).toBe('5:31');
    expect(fmtPace(352)).toBe('5:52');
    expect(fmtPace(59.6)).toBe('1:00');
    expect(fmtPace(0)).toBe('—');
  });


  it('fmtDuration renders mm:ss and h:mm:ss', () => {
    expect(fmtDuration(2900)).toBe('48:20');
    expect(fmtDuration(3730)).toBe('1:02:10');
    expect(fmtDuration(59)).toBe('0:59');
  });
});
