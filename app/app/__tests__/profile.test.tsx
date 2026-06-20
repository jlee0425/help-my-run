import React from 'react';
import { render, fireEvent, act } from '@testing-library/react-native';
import type { AthleteProfile } from '../../src/api/types';

const profile: AthleteProfile = {
  target_weekly_km: 20,
  progression_mode: 'build',
  zone2_ceiling_bpm: 150,
  threshold_bpm: 168,
  max_hr_bpm: 190,
  run_constraints_json: '{"crossfit_days":["Mon","Tue"]}',
  goal_text: 'Build cardio over time',
  updated_at: '2026-06-20T08:00:00Z',
};

const mockUpdateMutate = jest.fn();

// NOTE: profile.tsx imports nothing from expo-router, so no expo-router mock is needed.
jest.mock('../../src/api/hooks', () => ({
  useProfile: () => ({ data: profile, isPending: false, isError: false }),
  useUpdateProfile: () => ({ mutate: mockUpdateMutate, isPending: false }),
}));

import ProfileScreen from '../profile';

afterEach(() => {
  jest.clearAllMocks();
});

describe('ProfileScreen', () => {
  it('prefills inputs from the loaded profile', async () => {
    const { getByTestId } = await render(<ProfileScreen />);
    expect(getByTestId('input-target-km').props.value).toBe('20');
    expect(getByTestId('input-goal').props.value).toBe('Build cardio over time');
    expect(getByTestId('input-zone2').props.value).toBe('150');
    expect(getByTestId('input-threshold-bpm').props.value).toBe('168');
    expect(getByTestId('input-maxhr').props.value).toBe('190');
    expect(getByTestId('input-constraints').props.value).toBe('{"crossfit_days":["Mon","Tue"]}');
  });

  it('shows the active progression mode', async () => {
    const { getByTestId } = await render(<ProfileScreen />);
    expect(getByTestId('mode-build')).toBeTruthy();
    expect(getByTestId('mode-hold')).toBeTruthy();
  });

  it('saves an edited profile with parsed numeric fields and nulls for blanks', async () => {
    const { getByTestId } = await render(<ProfileScreen />);
    await act(async () => {
      fireEvent.changeText(getByTestId('input-target-km'), '25');
    });
    await act(async () => {
      fireEvent.press(getByTestId('mode-hold'));
    });
    await act(async () => {
      fireEvent.changeText(getByTestId('input-zone2'), '');
    });
    await act(async () => {
      fireEvent.changeText(getByTestId('input-goal'), 'Run a half');
    });
    await act(async () => {
      fireEvent.press(getByTestId('btn-save-profile'));
    });

    expect(mockUpdateMutate).toHaveBeenCalledTimes(1);
    const arg = mockUpdateMutate.mock.calls[0][0] as AthleteProfile;
    expect(arg.target_weekly_km).toBe(25);
    expect(arg.progression_mode).toBe('hold');
    expect(arg.zone2_ceiling_bpm).toBeNull();
    expect(arg.threshold_bpm).toBe(168);
    expect(arg.max_hr_bpm).toBe(190);
    expect(arg.goal_text).toBe('Run a half');
    expect(arg.run_constraints_json).toBe('{"crossfit_days":["Mon","Tue"]}');
  });
});
