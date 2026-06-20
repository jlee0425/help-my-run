import React from 'react';
import { render, fireEvent, act } from '@testing-library/react-native';
import type { Plan } from '../../src/api/types';

const plan: Plan = {
  id: 7, week_start: '2026-06-22', generated_at: '2026-06-20T08:05:12Z',
  fitness_summary: '~18 km/week, acute:chronic 1.05.',
  weekly_target_km: 25,
  days: [
    { date: '2026-06-22', dow: 'Mon', run_type: 'rest', distance_km: 0, pace_target: '', time_note: '', optional_if_cns: false, rationale: 'Heavy squat day; no run.' },
    { date: '2026-06-23', dow: 'Tue', run_type: 'easy', distance_km: 5, pace_target: '6:00/km', time_note: '~20:00 after CrossFit', optional_if_cns: true, rationale: 'Low leg load; easy double.' },
  ],
  week_rationale: 'Quality placed Thursday; long run Saturday.',
  one_flag: 'If Thursday skill work runs heavy, downgrade the tempo.',
};

const mockRegenerate = jest.fn();

// Mutable mock state so the cold-start case can swap hook return values without
// jest.resetModules() (which would fork the React instance under jest-expo and
// break the reconciler). The default state is the happy path.
const mockHookState: {
  plan: { data: Plan | undefined; isPending: boolean; isError: boolean };
  generate: { mutate: jest.Mock; isPending: boolean; isError: boolean; error: Error | null };
} = {
  plan: { data: plan, isPending: false, isError: false },
  generate: { mutate: mockRegenerate, isPending: false, isError: false, error: null },
};

jest.mock('expo-router', () => ({
  Stack: { Screen: () => null },
  useLocalSearchParams: () => ({ week: '2026-06-22' }),
}));

jest.mock('../../src/api/hooks', () => ({
  usePlan: () => mockHookState.plan,
  useGeneratePlan: () => mockHookState.generate,
}));

import PlanViewScreen from '../plan-view';

afterEach(() => {
  jest.clearAllMocks();
  mockHookState.plan = { data: plan, isPending: false, isError: false };
  mockHookState.generate = { mutate: mockRegenerate, isPending: false, isError: false, error: null };
});

describe('PlanViewScreen', () => {
  it('renders the fitness summary and weekly target', async () => {
    const { getByTestId } = await render(<PlanViewScreen />);
    expect(getByTestId('plan-fitness-summary').props.children).toContain('~18 km/week, acute:chronic 1.05.');
    expect(getByTestId('plan-weekly-target').props.children).toContain(25);
  });

  it('renders one card per planned day with type/distance/pace/time/optional', async () => {
    const { getByTestId } = await render(<PlanViewScreen />);
    expect(getByTestId('plan-day-2026-06-22')).toBeTruthy();
    const tue = getByTestId('plan-day-2026-06-23-detail').props.children.join('');
    expect(tue).toContain('5');
    expect(tue).toContain('6:00/km');
    expect(tue).toContain('~20:00 after CrossFit');
    expect(getByTestId('plan-day-2026-06-23-title').props.children.join('')).toContain('optional');
  });

  it('renders the week rationale and one flag', async () => {
    const { getByTestId } = await render(<PlanViewScreen />);
    expect(getByTestId('plan-week-rationale').props.children).toContain('Quality placed Thursday; long run Saturday.');
    expect(getByTestId('plan-one-flag').props.children).toContain('If Thursday skill work runs heavy, downgrade the tempo.');
  });

  it('regenerates the plan for the same week when Regenerate is pressed', async () => {
    const { getByTestId } = await render(<PlanViewScreen />);
    await act(async () => {
      fireEvent.press(getByTestId('btn-regenerate'));
    });
    expect(mockRegenerate).toHaveBeenCalledWith({ week_start: '2026-06-22' });
  });

  it('does NOT show the regenerate error line on the happy path', async () => {
    const { queryByTestId } = await render(<PlanViewScreen />);
    expect(queryByTestId('plan-regenerate-error')).toBeNull();
  });
});

// Cold-start 404: Regenerate with no stored CrossFit week for this week. The
// backend returns 404 ("no crossfit week for that week"); the view must surface a
// friendly, specific message rather than a generic failure. Swap the mutable hook
// state so useGeneratePlan reports a 404 error for this isolated case.
describe('PlanViewScreen — Regenerate cold-start 404', () => {
  it('surfaces a parse-a-photo hint when there is no CrossFit week for the week', async () => {
    mockHookState.plan = { data: undefined, isPending: false, isError: false };
    mockHookState.generate = {
      mutate: jest.fn(),
      isPending: false,
      isError: true,
      // apiPost rejects with an Error whose message carries the HTTP status + body.
      error: new Error('404: no crossfit week for that week'),
    };
    const { getByTestId } = await render(<PlanViewScreen />);
    expect(getByTestId('plan-regenerate-error').props.children).toContain(
      'No CrossFit week for this week — parse a photo first.',
    );
  });
});
