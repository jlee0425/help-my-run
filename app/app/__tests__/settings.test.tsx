import React from 'react';
import { render, fireEvent, act } from '@testing-library/react-native';
import type { AthleteProfile } from '../../src/api/types';

const profileData: AthleteProfile = {
  target_weekly_km: 20,
  progression_mode: 'build',
  zone2_ceiling_bpm: null,
  threshold_bpm: null,
  max_hr_bpm: null,
  run_constraints_json: '{}',
  goal_text: '',
  daily_run_time: '05:30',
  timezone: 'Asia/Seoul',
  agent_enabled: true,
  updated_at: '2026-06-20T05:00:00Z',
};

const mockProfileUpdateMutate = jest.fn();

const mockSave = jest.fn();
const mockSyncMutate = jest.fn();

jest.mock('expo-router', () => ({ Stack: { Screen: () => null } }));

jest.mock('../../src/api/settings', () => ({
  useSettings: () => ({
    baseUrl: 'http://localhost:8080',
    token: 'stored-token',
    loading: false,
    save: mockSave,
  }),
}));

jest.mock('../../src/api/hooks', () => ({
  useStatus: () => ({
    data: {
      garmin: { connected: false, last_synced_at: null, last_run_at: null, status: 'never', error: null },
      counts: { activities: 0, recovery_days: 0 },
    },
    isPending: false,
    isError: false,
  }),
  useSync: () => ({ mutate: mockSyncMutate, isPending: false }),
  useProfile: () => ({ data: profileData, isPending: false, isError: false }),
  useUpdateProfile: () => ({ mutate: mockProfileUpdateMutate, isPending: false }),
}));

import SettingsScreen from '../settings';

afterEach(() => {
  jest.clearAllMocks();
});

// render() is async in @testing-library/react-native v14 (React 19
// test-renderer), so each test awaits it and queries the returned result
// instead of the synchronous `screen` global. fireEvent interactions that
// trigger state updates are wrapped in `await act(...)` so the concurrent
// render flushes before assertions and the renderer is left clean for the
// next test.
describe('SettingsScreen', () => {
  it('renders inputs prefilled with stored config', async () => {
    const { getByTestId } = await render(<SettingsScreen />);
    expect(getByTestId('input-base-url').props.value).toBe('http://localhost:8080');
    expect(getByTestId('input-token').props.value).toBe('stored-token');
  });

  it('saves edited config when Save is pressed', async () => {
    const { getByTestId } = await render(<SettingsScreen />);
    await act(async () => {
      fireEvent.changeText(getByTestId('input-base-url'), 'http://10.0.0.5:8080');
    });
    await act(async () => {
      fireEvent.changeText(getByTestId('input-token'), 'new-token');
    });
    await act(async () => {
      fireEvent.press(getByTestId('btn-save'));
    });
    expect(mockSave).toHaveBeenCalledWith('http://10.0.0.5:8080', 'new-token');
  });

  it('shows the Garmin not-connected state', async () => {
    const { getByTestId } = await render(<SettingsScreen />);
    expect(getByTestId('garmin-status').props.children).toContain('Not connected');
  });

  it('triggers a sync when Sync now is pressed', async () => {
    const { getByTestId } = await render(<SettingsScreen />);
    await act(async () => {
      fireEvent.press(getByTestId('btn-sync'));
    });
    expect(mockSyncMutate).toHaveBeenCalledTimes(1);
  });

  it('prefills the daily run time, timezone, and agent toggle from the profile', async () => {
    const { getByTestId } = await render(<SettingsScreen />);
    expect(getByTestId('input-daily-run-time').props.value).toBe('05:30');
    expect(getByTestId('input-timezone').props.value).toBe('Asia/Seoul');
    // RN 0.85 Switch surfaces its state via props.value (no accessibilityState.checked).
    expect(getByTestId('toggle-agent-enabled').props.value).toBe(true);
  });

  it('saves the agent schedule with edited run time and toggled-off agent', async () => {
    const { getByTestId } = await render(<SettingsScreen />);
    await act(async () => { fireEvent.changeText(getByTestId('input-daily-run-time'), '06:15'); });
    await act(async () => { fireEvent.changeText(getByTestId('input-timezone'), 'UTC'); });
    // RN 0.85 Switch toggles via its onValueChange callback, not a press.
    await act(async () => { fireEvent(getByTestId('toggle-agent-enabled'), 'valueChange', false); });
    await act(async () => { fireEvent.press(getByTestId('btn-save-agent')); });
    expect(mockProfileUpdateMutate).toHaveBeenCalledTimes(1);
    const arg = mockProfileUpdateMutate.mock.calls[0][0] as AthleteProfile;
    expect(arg.daily_run_time).toBe('06:15');
    expect(arg.timezone).toBe('UTC');
    expect(arg.agent_enabled).toBe(false);
    expect(arg.target_weekly_km).toBe(20);
    expect(arg.progression_mode).toBe('build');
  });
});
