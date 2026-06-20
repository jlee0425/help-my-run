import React from 'react';
import { renderHook, waitFor, act } from '@testing-library/react-native';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

jest.mock('../client', () => ({
  apiGet: jest.fn(),
  apiPost: jest.fn(),
  apiPut: jest.fn(),
  apiUpload: jest.fn(),
}));

import { apiGet, apiPost, apiPut, apiUpload } from '../client';
import {
  useStatus,
  useActivities,
  useRecovery,
  useSync,
  useProfile,
  useUpdateProfile,
  useFitness,
  usePlan,
  useParseCrossfit,
  useGeneratePlan,
} from '../hooks';
import type {
  Status,
  ActivitiesResponse,
  RecoveryResponse,
  SyncResponse,
  AthleteProfile,
  Fitness,
  CrossFitWeek,
  Plan,
} from '../types';

const mockApiGet = apiGet as jest.MockedFunction<typeof apiGet>;
const mockApiPost = apiPost as jest.MockedFunction<typeof apiPost>;
const mockApiPut = apiPut as jest.MockedFunction<typeof apiPut>;
const mockApiUpload = apiUpload as jest.MockedFunction<typeof apiUpload>;

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

    await act(async () => {
      result.current.mutate();
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockApiPost).toHaveBeenCalledWith('/api/sync');
    expect(result.current.data).toEqual(data);
  });
});

describe('useProfile', () => {
  it('fetches /api/profile', async () => {
    const data: AthleteProfile = {
      target_weekly_km: 20, progression_mode: 'build',
      zone2_ceiling_bpm: null, threshold_bpm: null, max_hr_bpm: null,
      run_constraints_json: '{}', goal_text: 'Build cardio', updated_at: '2026-06-20T08:00:00Z',
    };
    mockApiGet.mockResolvedValue(data);
    const { result } = await renderHook(() => useProfile(), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockApiGet).toHaveBeenCalledWith('/api/profile');
    expect(result.current.data).toEqual(data);
  });
});

describe('useUpdateProfile', () => {
  it('PUTs /api/profile with the profile body', async () => {
    const profile: AthleteProfile = {
      target_weekly_km: 25, progression_mode: 'hold',
      zone2_ceiling_bpm: 150, threshold_bpm: 168, max_hr_bpm: 190,
      run_constraints_json: '{}', goal_text: 'Hold steady',
    };
    mockApiPut.mockResolvedValue(profile);
    const { result } = await renderHook(() => useUpdateProfile(), { wrapper: createWrapper() });
    await act(async () => { result.current.mutate(profile); });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockApiPut).toHaveBeenCalledWith('/api/profile', profile);
    expect(result.current.data).toEqual(profile);
  });
});

describe('useFitness', () => {
  it('fetches /api/fitness', async () => {
    const data: Fitness = {
      weekly_volume_km: 18.2, four_week_avg_km: 17.4, acute_chronic_ratio: 1.05,
      easy_pace: '6:00/km', threshold_pace: '5:05/km', recovery_trend: 'improving',
      safe_weekly_target_km: 20, is_cutback_week: false,
    };
    mockApiGet.mockResolvedValue(data);
    const { result } = await renderHook(() => useFitness(), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockApiGet).toHaveBeenCalledWith('/api/fitness');
    expect(result.current.data).toEqual(data);
  });
});

describe('usePlan', () => {
  it('fetches /api/plan with the week query param', async () => {
    const data: Plan = {
      week_start: '2026-06-22', fitness_summary: 's', weekly_target_km: 20,
      days: [], week_rationale: 'r', one_flag: 'f',
    };
    mockApiGet.mockResolvedValue(data);
    const { result } = await renderHook(() => usePlan('2026-06-22'), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockApiGet).toHaveBeenCalledWith('/api/plan?week=2026-06-22');
    expect(result.current.data).toEqual(data);
  });

  it('is disabled (does not fetch) when week is empty', async () => {
    const { result } = await renderHook(() => usePlan(''), { wrapper: createWrapper() });
    expect(result.current.fetchStatus).toBe('idle');
    expect(mockApiGet).not.toHaveBeenCalled();
  });
});

describe('useParseCrossfit', () => {
  it('uploads the image and returns the parsed week', async () => {
    const week: CrossFitWeek = { week_start: '2026-06-22', days: [] };
    mockApiUpload.mockResolvedValue(week);
    const { result } = await renderHook(() => useParseCrossfit(), { wrapper: createWrapper() });
    await act(async () => {
      result.current.mutate({ uri: 'file:///x.jpg', name: 'x.jpg', type: 'image/jpeg' });
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockApiUpload).toHaveBeenCalledWith('/api/crossfit/parse', {
      uri: 'file:///x.jpg', name: 'x.jpg', type: 'image/jpeg',
    });
    expect(result.current.data).toEqual(week);
  });
});

describe('useGeneratePlan', () => {
  it('POSTs /api/plan/generate with week_start + crossfit_week', async () => {
    const week: CrossFitWeek = { week_start: '2026-06-22', days: [] };
    const plan: Plan = {
      id: 7, week_start: '2026-06-22', generated_at: '2026-06-20T08:05:12Z',
      fitness_summary: 's', weekly_target_km: 20, days: [], week_rationale: 'r', one_flag: 'f',
    };
    mockApiPost.mockResolvedValue(plan);
    const { result } = await renderHook(() => useGeneratePlan(), { wrapper: createWrapper() });
    await act(async () => {
      result.current.mutate({ week_start: '2026-06-22', crossfit_week: week });
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockApiPost).toHaveBeenCalledWith('/api/plan/generate', {
      week_start: '2026-06-22', crossfit_week: week,
    });
    expect(result.current.data).toEqual(plan);
  });
});
