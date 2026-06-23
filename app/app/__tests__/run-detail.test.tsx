import React from 'react';
import { render, fireEvent, act } from '@testing-library/react-native';
import type { StreamAnalysis } from '../../src/api/types';

const analysis: StreamAnalysis = {
  activity_id: 14820001234, has_stream: true, has_hr: true,
  time_in_zone: [
    { zone: 1, seconds: 600, pct: 20 },
    { zone: 2, seconds: 1800, pct: 60 },
    { zone: 3, seconds: 600, pct: 20 },
  ],
  decoupling_pct: 4.2, pa_hr_first: 0.021, pa_hr_second: 0.020,
  zones: { z1_hi: 116, z2_hi: 145, z3_hi: 157.5, z4_hi: 170 },
  source: 'garmin', computed_at: '2026-06-22T07:00:00Z',
};

const mockFetch = jest.fn();

const mockHookState: {
  analysis: { data: StreamAnalysis | undefined; isPending: boolean; isError: boolean };
  fetch: { mutate: jest.Mock; isPending: boolean };
} = {
  analysis: { data: analysis, isPending: false, isError: false },
  fetch: { mutate: mockFetch, isPending: false },
};

jest.mock('expo-router', () => {
  const { Text: RNText } = require('react-native');
  return {
    Link: ({ children }: { children: React.ReactNode }) => <RNText>{children}</RNText>,
    Stack: { Screen: () => null },
    useLocalSearchParams: () => ({ id: '14820001234' }),
  };
});

jest.mock('../../src/api/hooks', () => ({
  useActivityAnalysis: () => mockHookState.analysis,
  useFetchStream: () => mockHookState.fetch,
}));

import RunDetailScreen from '../run/[id]';

afterEach(() => {
  jest.clearAllMocks();
  mockHookState.analysis = { data: analysis, isPending: false, isError: false };
  mockHookState.fetch = { mutate: mockFetch, isPending: false };
});

describe('RunDetailScreen — happy path', () => {
  it('renders one zone bar per ZoneTime', async () => {
    const { getByTestId, queryByTestId } = await render(<RunDetailScreen />);
    expect(getByTestId('zone-bar-1')).toBeTruthy();
    expect(getByTestId('zone-bar-2')).toBeTruthy();
    expect(getByTestId('zone-bar-3')).toBeTruthy();
    expect(queryByTestId('zone-bar-4')).toBeNull();
  });

  it('renders the decoupling value', async () => {
    const { getByTestId } = await render(<RunDetailScreen />);
    expect(getByTestId('decoupling-value').props.children).toContain('4.2');
  });

  it('does NOT show the fetch button or no-HR state on the happy path', async () => {
    const { queryByTestId } = await render(<RunDetailScreen />);
    expect(queryByTestId('btn-fetch-stream')).toBeNull();
    expect(queryByTestId('run-no-hr')).toBeNull();
  });
});

describe('RunDetailScreen — no HR', () => {
  it('shows the no-HR state, no bars, decoupling em-dash', async () => {
    mockHookState.analysis = {
      data: { ...analysis, has_hr: false, time_in_zone: [], decoupling_pct: null },
      isPending: false, isError: false,
    };
    const { getByTestId, queryByTestId } = await render(<RunDetailScreen />);
    expect(getByTestId('run-no-hr')).toBeTruthy();
    expect(queryByTestId('zone-bar-1')).toBeNull();
    expect(getByTestId('decoupling-value').props.children).toContain('—');
  });
});

describe('RunDetailScreen — not fetched', () => {
  it('shows the fetch button when has_stream is false and calls fetch on press', async () => {
    mockHookState.analysis = {
      data: { ...analysis, has_stream: false, has_hr: false, time_in_zone: [], decoupling_pct: null },
      isPending: false, isError: false,
    };
    const { getByTestId, queryByTestId } = await render(<RunDetailScreen />);
    expect(getByTestId('btn-fetch-stream')).toBeTruthy();
    expect(queryByTestId('zone-bar-2')).toBeNull();
    await act(async () => {
      fireEvent.press(getByTestId('btn-fetch-stream'));
    });
    expect(mockFetch).toHaveBeenCalledTimes(1);
  });

  it('disables the button and shows "Fetching…" while pending', async () => {
    mockHookState.analysis = {
      data: { ...analysis, has_stream: false, has_hr: false, time_in_zone: [], decoupling_pct: null },
      isPending: false, isError: false,
    };
    mockHookState.fetch = { mutate: mockFetch, isPending: true };
    const { getByText } = await render(<RunDetailScreen />);
    expect(getByText('Fetching…')).toBeTruthy();
  });
});

describe('RunDetailScreen — loading', () => {
  it('shows a loading state while pending', async () => {
    mockHookState.analysis = { data: undefined, isPending: true, isError: false };
    const { getByTestId } = await render(<RunDetailScreen />);
    expect(getByTestId('run-loading')).toBeTruthy();
  });
});

describe('RunDetailScreen — source badge', () => {
  it('shows "HR via Garmin .FIT" badge when source is garmin', async () => {
    mockHookState.analysis = {
      data: { ...analysis, source: 'garmin' },
      isPending: false, isError: false,
    };
    const { getByTestId } = await render(<RunDetailScreen />);
    const badge = getByTestId('source-badge');
    expect(badge).toBeTruthy();
    expect(badge.props.children).toBe('HR via Garmin .FIT');
  });

  it('does NOT show the source badge when source is empty', async () => {
    mockHookState.analysis = {
      data: { ...analysis, source: '' },
      isPending: false, isError: false,
    };
    const { queryByTestId } = await render(<RunDetailScreen />);
    expect(queryByTestId('source-badge')).toBeNull();
  });

  it('does NOT show the source badge when there is no HR (no zone section)', async () => {
    mockHookState.analysis = {
      data: { ...analysis, source: 'garmin', has_hr: false, time_in_zone: [], decoupling_pct: null },
      isPending: false, isError: false,
    };
    const { queryByTestId } = await render(<RunDetailScreen />);
    expect(queryByTestId('source-badge')).toBeNull();
  });
});
