import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router';
import { describe, expect, it, vi } from 'vitest';
import type { Today } from '../api/types';
import { TodayPage } from './TodayPage';

const mocks = vi.hoisted(() => ({
  today: null as Today | null,
  undoMutate: vi.fn(),
  agentMutate: vi.fn(),
}));

vi.mock('../api/hooks', () => ({
  useToday: () => ({ data: mocks.today, isLoading: false }),
  useRecovery: () => ({ data: [] }),
  useActivities: () => ({
    data: [
      {
        activity_id: 42,
        name: 'Threshold · 6×800m',
        type: 'running',
        sport_type: null,
        start_time: '2026-07-04T06:00:00Z',
        start_time_local: null,
        distance_m: 9400,
        moving_time_s: 2900,
        elapsed_time_s: 2900,
        avg_hr: 156,
        max_hr: 176,
        avg_speed: 3.24,
        max_speed: 4,
        avg_cadence: null,
        elevation_gain_m: null,
      },
    ],
  }),
  useProgress: () => ({ data: { weeks: 12, signals: [], enough_data: false } }),
  useUndoToday: () => ({ mutate: mocks.undoMutate, isPending: false }),
  useAgentRun: () => ({ mutate: mocks.agentMutate, isPending: false, error: null }),
}));

const AMBER_TODAY: Today = {
  date: '2026-07-09',
  readiness_color: 'amber',
  drivers: {
    date: '2026-07-09',
    sleep_hours: 6.0,
    sleep_score: 64,
    hrv_last_night_ms: 48,
    hrv_baseline_ms: 61,
    hrv_delta_pct: -13,
    rhr_last_night: 54,
    rhr_baseline: 49,
    rhr_delta_bpm: 5,
    body_battery_high: 41,
    recovery_trend: 'declining',
    data_complete: true,
  },
  reasons: ['HRV -13% vs baseline'],
  action: 'SOFTEN',
  original_session: {
    date: '2026-07-09',
    dow: 'Wed',
    run_type: 'threshold',
    distance_km: 8,
    pace_target: '4:15/km',
    time_note: '6×800m',
    optional_if_cns: false,
    rationale: '',
  },
  effective_session: {
    date: '2026-07-09',
    dow: 'Wed',
    run_type: 'easy',
    distance_km: 8,
    pace_target: '@ 5:35/km · HR < 148',
    time_note: '',
    optional_if_cns: false,
    rationale: '',
  },
  rationale: 'Softened today to easy. HRV 48 (−13 vs 7-day).',
  source: 'ai',
  stale: false,
};

function renderPage() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: 0 } } });
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <TodayPage />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe('TodayPage', () => {
  it('renders the verdict, drivers, softened chip, session and signals', () => {
    mocks.today = AMBER_TODAY;
    renderPage();
    expect(screen.getByText('AMBER')).toBeInTheDocument();
    expect(screen.getByText('READINESS')).toBeInTheDocument();
    expect(screen.getByText(/softened from threshold · 8 km/)).toBeInTheDocument();
    expect(screen.getByText('Easy · 8 km')).toBeInTheDocument();
    expect(screen.getByText('Coach reshaped your day.')).toBeInTheDocument();
    expect(screen.getByText('// SIGNALS')).toBeInTheDocument();
    expect(screen.getByText('LOAD a:c')).toBeInTheDocument();
    expect(screen.getByText(/LAST SESSION/)).toBeInTheDocument();
  });

  it('undo keeps the original session', async () => {
    mocks.today = AMBER_TODAY;
    renderPage();
    await userEvent.click(screen.getByText(/undo — keep the original session/));
    expect(mocks.undoMutate).toHaveBeenCalledOnce();
  });

  it('renders rest day without session buttons chip', () => {
    mocks.today = {
      ...AMBER_TODAY,
      action: 'REST_DAY',
      original_session: null,
      effective_session: null,
      rationale: 'Rest day — readiness low, stay recovered.',
    };
    renderPage();
    expect(screen.getByText('Rest day')).toBeInTheDocument();
    expect(screen.queryByText(/softened from/)).not.toBeInTheDocument();
  });

  it('offers Run coach now when there is no decision yet', async () => {
    mocks.today = null;
    renderPage();
    expect(screen.getByText('No decision yet today.')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: /Run coach now/ }));
    expect(mocks.agentMutate).toHaveBeenCalledWith({ force: true });
  });
});
