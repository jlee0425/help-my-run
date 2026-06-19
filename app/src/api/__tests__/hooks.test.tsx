import React from 'react';
import { renderHook, waitFor, act } from '@testing-library/react-native';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

jest.mock('../client', () => ({
  apiGet: jest.fn(),
  apiPost: jest.fn(),
}));

import { apiGet, apiPost } from '../client';
import { useStatus, useActivities, useRecovery, useSync } from '../hooks';
import type { Status, ActivitiesResponse, RecoveryResponse, SyncResponse } from '../types';

const mockApiGet = apiGet as jest.MockedFunction<typeof apiGet>;
const mockApiPost = apiPost as jest.MockedFunction<typeof apiPost>;

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
}

afterEach(() => {
  jest.clearAllMocks();
});

describe('useStatus', () => {
  it('fetches /api/status', async () => {
    const data: Status = {
      strava: { connected: true, athlete_id: 1, last_synced_at: null, last_run_at: null, status: 'ok', error: null },
      garmin: { connected: false, last_synced_at: null, last_run_at: null, status: 'never', error: null },
      counts: { activities: 5, recovery_days: 3 },
    };
    mockApiGet.mockResolvedValue(data);

    const { result } = await renderHook(() => useStatus(), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockApiGet).toHaveBeenCalledWith('/api/status');
    expect(result.current.data).toEqual(data);
  });
});

describe('useActivities', () => {
  it('fetches /api/activities with default limit 30', async () => {
    const data: ActivitiesResponse = { activities: [] };
    mockApiGet.mockResolvedValue(data);

    const { result } = await renderHook(() => useActivities(), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockApiGet).toHaveBeenCalledWith('/api/activities?limit=30');
    expect(result.current.data).toEqual(data);
  });

  it('fetches /api/activities with an explicit limit', async () => {
    mockApiGet.mockResolvedValue({ activities: [] });

    const { result } = await renderHook(() => useActivities(10), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockApiGet).toHaveBeenCalledWith('/api/activities?limit=10');
  });
});

describe('useRecovery', () => {
  it('fetches /api/recovery with default days 30', async () => {
    const data: RecoveryResponse = { recovery: [] };
    mockApiGet.mockResolvedValue(data);

    const { result } = await renderHook(() => useRecovery(), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockApiGet).toHaveBeenCalledWith('/api/recovery?days=30');
    expect(result.current.data).toEqual(data);
  });

  it('fetches /api/recovery with an explicit days value', async () => {
    mockApiGet.mockResolvedValue({ recovery: [] });

    const { result } = await renderHook(() => useRecovery(7), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockApiGet).toHaveBeenCalledWith('/api/recovery?days=7');
  });
});

describe('useSync', () => {
  it('POSTs /api/sync and returns per-source results', async () => {
    const data: SyncResponse = {
      strava: { status: 'ok', synced: 3, error: null },
      garmin: { status: 'ok', synced: 5, error: null },
    };
    mockApiPost.mockResolvedValue(data);

    const { result } = await renderHook(() => useSync(), { wrapper: createWrapper() });

    act(() => {
      result.current.mutate();
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockApiPost).toHaveBeenCalledWith('/api/sync');
    expect(result.current.data).toEqual(data);
  });
});
