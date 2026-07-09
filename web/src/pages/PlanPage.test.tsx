import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router';
import { describe, expect, it, vi } from 'vitest';
import type { CrossFitWeek, Plan } from '../api/types';
import { mondayOf, PlanPage, shiftWeek } from './PlanPage';

const mocks = vi.hoisted(() => ({
  plan: null as Plan | null,
  parseMutate: vi.fn(),
  generateMutate: vi.fn(),
}));

vi.mock('../api/hooks', () => ({
  usePlan: () => ({ data: mocks.plan, isLoading: false }),
  useCrossfitParse: () => ({ mutate: mocks.parseMutate, isPending: false, error: null }),
  usePlanGenerate: () => ({ mutate: mocks.generateMutate, isPending: false, error: null }),
}));

const PLAN: Plan = {
  id: 1,
  week_start: '2026-07-06',
  generated_at: 'x',
  fitness_summary: 's',
  weekly_target_km: 24,
  week_rationale: 'Build week around Thursday heavy squats.',
  one_flag: 'PROTECT FRIDAY',
  days: [
    {
      date: '2026-07-06',
      dow: 'Mon',
      run_type: 'rest',
      distance_km: 0,
      pace_target: '',
      time_note: '',
      optional_if_cns: false,
      rationale: '',
    },
    {
      date: '2026-07-08',
      dow: 'Wed',
      run_type: 'easy',
      distance_km: 8,
      pace_target: '5:35/km',
      time_note: 'after CrossFit',
      optional_if_cns: true,
      rationale: '',
    },
  ],
};

function renderPage() {
  return render(
    <MemoryRouter>
      <PlanPage />
    </MemoryRouter>,
  );
}

describe('week math', () => {
  it('mondayOf returns the ISO Monday', () => {
    expect(mondayOf(new Date('2026-07-09T10:00:00Z'))).toBe('2026-07-06'); // Thu -> Mon
    expect(mondayOf(new Date('2026-07-06T00:00:00Z'))).toBe('2026-07-06'); // Mon -> itself
    expect(mondayOf(new Date('2026-07-12T23:00:00Z'))).toBe('2026-07-06'); // Sun -> prior Mon
  });

  it('shiftWeek moves by whole weeks', () => {
    expect(shiftWeek('2026-07-06', -1)).toBe('2026-06-29');
    expect(shiftWeek('2026-07-06', 1)).toBe('2026-07-13');
  });
});

describe('PlanPage', () => {
  it('renders days, rationale, optional chip', () => {
    mocks.plan = PLAN;
    renderPage();
    expect(screen.getByText('Easy · 8 km')).toBeInTheDocument();
    expect(screen.getByText('Rest')).toBeInTheDocument();
    expect(screen.getByText('optional')).toBeInTheDocument();
    expect(screen.getByText(/Build week around Thursday/)).toBeInTheDocument();
    expect(screen.getByText(/TARGET 24 KM · PROTECT FRIDAY/)).toBeInTheDocument();
  });

  it('shows empty state and drives upload → parse → generate', async () => {
    mocks.plan = null;
    mocks.parseMutate.mockImplementation(
      (_vars: unknown, opts?: { onSuccess?: (w: CrossFitWeek) => void }) => {
        opts?.onSuccess?.({
          week_start: '2026-07-06',
          days: [
            {
              date: '2026-07-06',
              dow: 'Mon',
              has_crossfit: true,
              focus: 'squats',
              cns_load: 'high',
              leg_load: 'high',
              notes: '',
            },
          ],
        });
      },
    );
    renderPage();
    expect(screen.getByText(/No plan for this week yet/)).toBeInTheDocument();

    const file = new File(['img'], 'schedule.jpg', { type: 'image/jpeg' });
    fireEvent.change(screen.getByTestId('cf-file'), { target: { files: [file] } });
    await waitFor(() => expect(mocks.parseMutate).toHaveBeenCalled());
    expect(screen.getByText(/MON · squats/)).toBeInTheDocument();

    await userEvent.click(screen.getByRole('button', { name: 'Generate plan' }));
    expect(mocks.generateMutate).toHaveBeenCalledWith(
      expect.objectContaining({ crossfitWeek: expect.objectContaining({ week_start: '2026-07-06' }) }),
    );
  });

  it('week nav shifts the week label', async () => {
    mocks.plan = PLAN;
    renderPage();
    const before = screen.getByText(mondayOf(new Date()));
    expect(before).toBeInTheDocument();
    await userEvent.click(screen.getByLabelText('Previous week'));
    expect(screen.getByText(shiftWeek(mondayOf(new Date()), -1))).toBeInTheDocument();
  });
});
