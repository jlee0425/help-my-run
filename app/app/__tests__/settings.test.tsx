import React from 'react';
import { render, fireEvent, act } from '@testing-library/react-native';

const mockSave = jest.fn();
const mockConnectMutate = jest.fn();
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
      strava: { connected: true, athlete_id: 1, last_synced_at: null, last_run_at: null, status: 'ok', error: null },
      garmin: { connected: false, last_synced_at: null, last_run_at: null, status: 'never', error: null },
      counts: { activities: 0, recovery_days: 0 },
    },
    isPending: false,
    isError: false,
  }),
  useSync: () => ({ mutate: mockSyncMutate, isPending: false }),
  useConnectStrava: () => ({ mutate: mockConnectMutate, isPending: false }),
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

  it('starts Strava connect when Connect is pressed', async () => {
    const { getByTestId } = await render(<SettingsScreen />);
    await act(async () => {
      fireEvent.press(getByTestId('btn-strava-connect'));
    });
    expect(mockConnectMutate).toHaveBeenCalledTimes(1);
  });

  it('shows the Strava connected state', async () => {
    const { getByTestId } = await render(<SettingsScreen />);
    expect(getByTestId('strava-status').props.children).toContain('Connected');
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
});
