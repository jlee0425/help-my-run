import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { describe, expect, it, vi } from 'vitest';
import type { ProgressReport } from '../api/types';
import { TrendsPage } from './TrendsPage';

const mocks = vi.hoisted(() => ({
  progress: null as ProgressReport | null,
}));

vi.mock('../api/hooks', () => ({
  useProgress: () => ({ data: mocks.progress, isLoading: false }),
  useActivities: () => ({ data: [] }),
  useRecovery: () => ({ data: [] }),
}));

function sig(key: string, series: (number | null)[], lowerIsBetter: boolean) {
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
    direction: (delta === null || delta === 0 ? 'flat' : delta > 0 ? 'up' : 'down') as
      | 'up'
      | 'down'
      | 'flat',
    lower_is_better: lowerIsBetter,
    series,
  };
}

describe('TrendsPage', () => {
  it('renders hero pace chart, delta chips and minis', () => {
    mocks.progress = {
      weeks: 12,
      generated_at: 'x',
      enough_data: true,
      signals: [
        sig('pace_at_hr', [352, 347, 343, 338, 331], true),
        sig('hrv_baseline', [55, 58, 61], false),
        sig('resting_hr', [53, 51, 49], true),
      ],
    };
    render(
      <MemoryRouter>
        <TrendsPage />
      </MemoryRouter>,
    );
    expect(screen.getByText('Same heart rate, faster running.')).toBeInTheDocument();
    expect(screen.getByText('5:31')).toBeInTheDocument();
    expect(screen.getByText(/12 wks ago · 5:52/)).toBeInTheDocument();
    expect(screen.getByText('▼21s')).toBeInTheDocument();
    expect(screen.getByText('HRV BASELINE')).toBeInTheDocument();
    expect(screen.getAllByText('RESTING HR').length).toBeGreaterThanOrEqual(1); // chip + mini card
    // no activities -> no load card; no sleep data -> no link card
    expect(screen.queryByText(/SLEEP → PACE LINK/)).not.toBeInTheDocument();
    expect(screen.queryByText(/WEEKLY LOAD/)).not.toBeInTheDocument();
  });

  it('shows the empty state when there is not enough data', () => {
    mocks.progress = { weeks: 12, generated_at: 'x', enough_data: false, signals: [] };
    render(
      <MemoryRouter>
        <TrendsPage />
      </MemoryRouter>,
    );
    expect(screen.getByText('Not enough data yet.')).toBeInTheDocument();
  });
});
