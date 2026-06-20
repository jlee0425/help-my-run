import React from 'react';
import { render, fireEvent, act, waitFor } from '@testing-library/react-native';
import type { CrossFitWeek, Plan } from '../../src/api/types';

const parsedWeek: CrossFitWeek = {
  week_start: '2026-06-22',
  days: [
    { date: '2026-06-22', dow: 'Mon', has_crossfit: true, focus: 'Back squat 5x5', cns_load: 'high', leg_load: 'high', notes: 'Heavy legs' },
    { date: '2026-06-23', dow: 'Tue', has_crossfit: true, focus: 'Row intervals', cns_load: 'med', leg_load: 'low', notes: '' },
  ],
};

const generatedPlan: Plan = {
  id: 7, week_start: '2026-06-22', generated_at: '2026-06-20T08:05:12Z',
  fitness_summary: 's', weekly_target_km: 20,
  days: [
    { date: '2026-06-22', dow: 'Mon', run_type: 'rest', distance_km: 0, pace_target: '', time_note: '', optional_if_cns: false, rationale: 'Heavy squat day.' },
  ],
  week_rationale: 'r', one_flag: 'f',
};

const mockPickFromLibrary = jest.fn();
const mockTakePhoto = jest.fn();
const mockToUploadFile = jest.fn();
const mockParseMutateAsync = jest.fn();
const mockGenerateMutate = jest.fn();

// NOTE: plan.tsx imports nothing from expo-router, so no expo-router mock is needed.
jest.mock('../../src/lib/imagePicker', () => ({
  pickFromLibrary: (...a: unknown[]) => mockPickFromLibrary(...a),
  takePhoto: (...a: unknown[]) => mockTakePhoto(...a),
  toUploadFile: (...a: unknown[]) => mockToUploadFile(...a),
}));

jest.mock('../../src/api/hooks', () => ({
  useParseCrossfit: () => ({ mutateAsync: mockParseMutateAsync, isPending: false }),
  useGeneratePlan: () => ({ mutate: mockGenerateMutate, isPending: false, data: undefined }),
}));

import PlanScreen from '../plan';

afterEach(() => {
  jest.clearAllMocks();
});

describe('PlanScreen', () => {
  it('renders the pick/take photo buttons', async () => {
    const { getByTestId } = await render(<PlanScreen />);
    expect(getByTestId('btn-pick-photo')).toBeTruthy();
    expect(getByTestId('btn-take-photo')).toBeTruthy();
  });

  it('parses a picked photo and renders editable per-day cards', async () => {
    mockPickFromLibrary.mockResolvedValue({ uri: 'file:///c.jpg', mimeType: 'image/jpeg', fileName: 'c.jpg' });
    mockToUploadFile.mockReturnValue({ uri: 'file:///c.jpg', name: 'c.jpg', type: 'image/jpeg' });
    mockParseMutateAsync.mockResolvedValue(parsedWeek);

    const { getByTestId } = await render(<PlanScreen />);
    await act(async () => {
      fireEvent.press(getByTestId('btn-pick-photo'));
    });

    await waitFor(() => expect(getByTestId('cf-day-2026-06-22')).toBeTruthy());
    expect(mockParseMutateAsync).toHaveBeenCalledWith({ uri: 'file:///c.jpg', name: 'c.jpg', type: 'image/jpeg' });
    expect(getByTestId('cf-focus-2026-06-22').props.value).toBe('Back squat 5x5');
  });

  it('edits a day focus + CNS load and generates with the edited week', async () => {
    mockPickFromLibrary.mockResolvedValue({ uri: 'file:///c.jpg', mimeType: 'image/jpeg', fileName: 'c.jpg' });
    mockToUploadFile.mockReturnValue({ uri: 'file:///c.jpg', name: 'c.jpg', type: 'image/jpeg' });
    mockParseMutateAsync.mockResolvedValue(parsedWeek);
    mockGenerateMutate.mockReturnValue(undefined);

    const { getByTestId } = await render(<PlanScreen />);
    await act(async () => {
      fireEvent.press(getByTestId('btn-pick-photo'));
    });
    await waitFor(() => expect(getByTestId('cf-day-2026-06-22')).toBeTruthy());

    await act(async () => {
      fireEvent.changeText(getByTestId('cf-focus-2026-06-22'), 'Edited focus');
    });
    await act(async () => {
      fireEvent.press(getByTestId('cf-cns-2026-06-22-low'));
    });
    await act(async () => {
      fireEvent.press(getByTestId('btn-generate'));
    });

    expect(mockGenerateMutate).toHaveBeenCalledTimes(1);
    const arg = mockGenerateMutate.mock.calls[0][0];
    expect(arg.week_start).toBe('2026-06-22');
    expect(arg.crossfit_week.days[0].focus).toBe('Edited focus');
    expect(arg.crossfit_week.days[0].cns_load).toBe('low');
  });

  it('does not show the generate button before a week is parsed', async () => {
    const { queryByTestId } = await render(<PlanScreen />);
    expect(queryByTestId('btn-generate')).toBeNull();
  });
});
