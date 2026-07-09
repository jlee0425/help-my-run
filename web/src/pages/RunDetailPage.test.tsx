import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router';
import { describe, expect, it, vi } from 'vitest';
import type { StreamAnalysis } from '../api/types';
import { onTarget, RunDetailPage } from './RunDetailPage';

const mocks = vi.hoisted(() => ({
  analysis: undefined as StreamAnalysis | undefined,
  fetchMutate: vi.fn(),
}));

vi.mock('../api/hooks', () => ({
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
        avg_speed: 9400 / 2900,
        max_speed: 4,
        avg_cadence: null,
        elevation_gain_m: null,
      },
    ],
  }),
  useAnalysis: () => ({ data: mocks.analysis }),
  useFetchStream: () => ({ mutate: mocks.fetchMutate, isPending: false, error: null }),
}));

const FULL_ANALYSIS: StreamAnalysis = {
  activity_id: 42,
  has_stream: true,
  has_hr: true,
  time_in_zone: [
    { zone: 1, seconds: 174, pct: 6 },
    { zone: 2, seconds: 870, pct: 30 },
    { zone: 3, seconds: 522, pct: 18 },
    { zone: 4, seconds: 1102, pct: 38 },
    { zone: 5, seconds: 232, pct: 8 },
  ],
  decoupling_pct: 4.2,
  pa_hr_first: 1.91,
  pa_hr_second: 1.84,
  zones: { z1_hi: 130, z2_hi: 148, z3_hi: 160, z4_hi: 172 },
  source: 'garmin_fit',
  computed_at: 'x',
};

function renderPage() {
  return render(
    <MemoryRouter initialEntries={['/runs/42']}>
      <Routes>
        <Route path="/runs/:id" element={<RunDetailPage />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe('RunDetailPage', () => {
  it('renders stats, zone band, drift and the on-target pill', () => {
    mocks.analysis = FULL_ANALYSIS;
    renderPage();
    expect(screen.getByText('Threshold · 6×800m')).toBeInTheDocument();
    expect(screen.getByText('9.4')).toBeInTheDocument(); // km
    expect(screen.getByText('48:20')).toBeInTheDocument(); // time
    expect(screen.getByText('5:09')).toBeInTheDocument(); // pace from avg_speed (308.5 s/km rounds up)
    expect(screen.getByText('156')).toBeInTheDocument(); // hr
    expect(screen.getByText('✓ on target')).toBeInTheDocument();
    expect(screen.getByText('Z4 38%')).toBeInTheDocument();
    expect(screen.getByText(/PA:HR DRIFT 4.2%/)).toBeInTheDocument();
  });

  it('offers stream fetch when no stream exists', async () => {
    mocks.analysis = { ...FULL_ANALYSIS, has_stream: false, decoupling_pct: null, time_in_zone: [] };
    renderPage();
    expect(screen.getByText('No stream data fetched for this run yet.')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Fetch stream' }));
    expect(mocks.fetchMutate).toHaveBeenCalledWith(42);
    expect(screen.queryByText('✓ on target')).not.toBeInTheDocument();
  });
});

describe('onTarget', () => {
  it('requires computed decoupling under 8%', () => {
    expect(onTarget(FULL_ANALYSIS)).toBe(true);
    expect(onTarget({ ...FULL_ANALYSIS, decoupling_pct: 9.1 })).toBe(false);
    expect(onTarget({ ...FULL_ANALYSIS, decoupling_pct: null })).toBe(false);
    expect(onTarget(undefined)).toBe(false);
  });
});
