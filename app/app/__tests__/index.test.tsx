import React from 'react';
import { Text } from 'react-native';
import { render } from '@testing-library/react-native';
import type { Status, ActivitiesResponse, RecoveryResponse } from '../../src/api/types';

// expo-router's <Link> renders its text children inside a pressable <Text> in
// real usage; the mock mirrors that so React Native's "strings must be inside
// <Text>" invariant is satisfied for the "Settings" link label.
jest.mock('expo-router', () => {
  const { Text: RNText } = require('react-native');
  return {
    Link: ({ children }: { children: React.ReactNode }) => <RNText>{children}</RNText>,
    Stack: { Screen: () => null },
  };
});

const statusData: Status = {
  strava: { connected: true, athlete_id: 1, last_synced_at: '2026-06-19T05:00:30Z', last_run_at: '2026-06-19T05:00:30Z', status: 'ok', error: null },
  garmin: { connected: true, last_synced_at: '2026-06-19T05:00:42Z', last_run_at: '2026-06-19T05:00:42Z', status: 'ok', error: null },
  counts: { activities: 42, recovery_days: 30 },
};

const activitiesData: ActivitiesResponse = {
  activities: [
    {
      strava_id: 14820001234, name: 'Morning Run', type: 'Run', sport_type: 'Run',
      start_time: '2026-06-18T06:12:00Z', start_time_local: '2026-06-18T08:12:00',
      distance_m: 10240.5, moving_time_s: 3120, elapsed_time_s: 3200,
      avg_hr: 152.3, max_hr: 171, avg_speed: 3.28, max_speed: 4.91,
      avg_cadence: 86.5, elevation_gain_m: 84.0,
    },
    {
      strava_id: 14820009999, name: 'Evening Jog', type: 'Run', sport_type: 'Run',
      start_time: '2026-06-17T18:00:00Z', start_time_local: '2026-06-17T20:00:00',
      distance_m: 5000, moving_time_s: 1500, elapsed_time_s: 1520,
      avg_hr: null, max_hr: null, avg_speed: null, max_speed: null,
      avg_cadence: null, elevation_gain_m: null,
    },
  ],
};

const recoveryData: RecoveryResponse = {
  recovery: [
    {
      date: '2026-06-18',
      sleep: { duration_s: 27000, deep_s: 6300, light_s: 14400, rem_s: 5400, awake_s: 900, score: 82 },
      hrv: { last_night_avg_ms: 48, status: 'BALANCED' },
      body_battery: { charged: 62, drained: 78, high: 91, low: 14 },
      rhr: { resting_hr: 47 },
    },
    {
      date: '2026-06-17',
      sleep: { duration_s: 25800, deep_s: 5400, light_s: 13800, rem_s: 4800, awake_s: 1800, score: 71 },
      hrv: null,
      body_battery: { charged: 58, drained: 80, high: 86, low: 12 },
      rhr: { resting_hr: 49 },
    },
  ],
};

jest.mock('../../src/api/hooks', () => ({
  useStatus: () => ({ data: statusData, isPending: false, isError: false }),
  useActivities: () => ({ data: activitiesData, isPending: false, isError: false }),
  useRecovery: () => ({ data: recoveryData, isPending: false, isError: false }),
}));

import HomeScreen from '../index';

// render() is async in @testing-library/react-native v14 (React 19
// test-renderer), so each test awaits it and queries the returned result.
describe('HomeScreen', () => {
  it('renders connection status for both sources', async () => {
    const { getByTestId } = await render(<HomeScreen />);
    expect(getByTestId('home-strava-status').props.children).toContain('Connected');
    expect(getByTestId('home-garmin-status').props.children).toContain('Connected');
  });

  it('renders the activity + recovery counts', async () => {
    const { getByTestId } = await render(<HomeScreen />);
    expect(getByTestId('count-activities').props.children).toContain(42);
    expect(getByTestId('count-recovery').props.children).toContain(30);
  });

  it('renders one row per recent run by name', async () => {
    const { getByText } = await render(<HomeScreen />);
    expect(getByText('Morning Run')).toBeTruthy();
    expect(getByText('Evening Jog')).toBeTruthy();
  });

  it('renders one row per recent recovery day by date', async () => {
    const { getByText } = await render(<HomeScreen />);
    expect(getByText('2026-06-18')).toBeTruthy();
    expect(getByText('2026-06-17')).toBeTruthy();
  });

  it('renders navigation links to Plan and Profile', async () => {
    const { getByText } = await render(<HomeScreen />);
    expect(getByText('Plan my week')).toBeTruthy();
    expect(getByText('Profile')).toBeTruthy();
  });
});
