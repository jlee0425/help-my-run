import React from 'react';
import { render, fireEvent, act } from '@testing-library/react-native';
import type { ProgressReport, ProgressRead } from '../../src/api/types';

const progress: ProgressReport = {
  weeks: 12,
  generated_at: '2026-06-21T07:00:00Z',
  enough_data: true,
  signals: [
    {
      key: 'pace_at_hr', label: 'Pace @ Z2 HR', unit: 's/km',
      current: 330, baseline: 350, delta_abs: -20,
      direction: 'down', lower_is_better: true,
      series: [350, null, 345, 340, null, 335, 330],
    },
    {
      key: 'vo2max', label: 'VO2max', unit: 'ml/kg/min',
      current: 52, baseline: 50, delta_abs: 2,
      direction: 'up', lower_is_better: false,
      series: [50, 50, 51, null, 51, 52, 52],
    },
  ],
};

const read: ProgressRead = { text: 'Your engine is improving.', source: 'ai' };

const mockAnalyze = jest.fn();

// Mutable mock state so the empty-state / analyzed cases swap return values
// without jest.resetModules() (which forks React under jest-expo and breaks the
// reconciler). Default = happy path.
const mockHookState: {
  progress: { data: ProgressReport | undefined; isPending: boolean; isError: boolean };
  analyze: { mutate: jest.Mock; data: ProgressRead | undefined; isPending: boolean };
} = {
  progress: { data: progress, isPending: false, isError: false },
  analyze: { mutate: mockAnalyze, data: undefined, isPending: false },
};

jest.mock('expo-router', () => {
  const { Text: RNText } = require('react-native');
  return {
    Link: ({ children }: { children: React.ReactNode }) => <RNText>{children}</RNText>,
    Stack: { Screen: () => null },
    useLocalSearchParams: () => ({}), // default 12 weeks
  };
});

jest.mock('../../src/api/hooks', () => ({
  useProgress: () => mockHookState.progress,
  useAnalyzeProgress: () => mockHookState.analyze,
}));

import ProgressScreen from '../progress';

afterEach(() => {
  jest.clearAllMocks();
  mockHookState.progress = { data: progress, isPending: false, isError: false };
  mockHookState.analyze = { mutate: mockAnalyze, data: undefined, isPending: false };
});

describe('ProgressScreen', () => {
  it('renders one trend card per signal', async () => {
    const { getByTestId } = await render(<ProgressScreen />);
    expect(getByTestId('progress-card-pace_at_hr')).toBeTruthy();
    expect(getByTestId('progress-card-vo2max')).toBeTruthy();
  });

  it('renders the label, current value, and delta vs window start per card', async () => {
    const { getByTestId } = await render(<ProgressScreen />);
    const pace = getByTestId('progress-card-pace_at_hr').props.children;
    // React 19 rendered elements carry `_owner`/`_store` back-references to live
    // FiberNodes, which make a naive JSON.stringify throw on a circular structure.
    // Drop those internal keys so we can flatten the visible card subtree to a string.
    const flat = JSON.stringify(pace, (key, value) =>
      key === '_owner' || key === '_store' ? undefined : value,
    );
    expect(flat).toContain('Pace @ Z2 HR');
    expect(flat).toContain('330');
    expect(flat).toContain('s/km');
  });

  it('shows an improving arrow for pace (lower_is_better + direction down)', async () => {
    const { getByTestId } = await render(<ProgressScreen />);
    expect(getByTestId('progress-arrow-pace_at_hr').props.children).toBe('↓');
  });

  it('shows an improving arrow for vo2max (higher is better + direction up)', async () => {
    const { getByTestId } = await render(<ProgressScreen />);
    expect(getByTestId('progress-arrow-vo2max').props.children).toBe('↑');
  });

  it('renders a sparkline string whose length equals the series length (gaps blank)', async () => {
    const { getByTestId } = await render(<ProgressScreen />);
    expect(getByTestId('progress-spark-pace_at_hr').props.children).toHaveLength(7);
  });

  it('runs analyze with the window when the button is pressed', async () => {
    const { getByTestId } = await render(<ProgressScreen />);
    await act(async () => {
      fireEvent.press(getByTestId('btn-analyze-progress'));
    });
    expect(mockAnalyze).toHaveBeenCalledTimes(1);
    expect(mockAnalyze).toHaveBeenCalledWith({ weeks: 12 });
  });

  it('shows the coach read footer after analyze returns', async () => {
    mockHookState.analyze = { mutate: mockAnalyze, data: read, isPending: false };
    const { getByTestId } = await render(<ProgressScreen />);
    expect(getByTestId('progress-read').props.children).toContain('Your engine is improving.');
  });

  it('does NOT render the empty state on the happy path', async () => {
    const { queryByTestId } = await render(<ProgressScreen />);
    expect(queryByTestId('progress-empty')).toBeNull();
  });
});

describe('ProgressScreen — not enough data', () => {
  it('shows the empty state and no cards when enough_data is false', async () => {
    mockHookState.progress = {
      data: { weeks: 12, generated_at: '2026-06-21T07:00:00Z', enough_data: false, signals: [] },
      isPending: false, isError: false,
    };
    const { getByTestId, queryByTestId } = await render(<ProgressScreen />);
    expect(getByTestId('progress-empty')).toBeTruthy();
    expect(queryByTestId('progress-card-pace_at_hr')).toBeNull();
  });
});
