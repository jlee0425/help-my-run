import React from 'react';
import { renderHook, waitFor, act } from '@testing-library/react-native';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

jest.mock('../client', () => ({
  apiGet: jest.fn(),
  apiPost: jest.fn(),
  apiPut: jest.fn(),
  apiDelete: jest.fn(),
  apiUpload: jest.fn(),
}));

import { apiGet, apiPost, apiPut, apiDelete, apiUpload } from '../client';
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
  useToday,
  useUndoToday,
  useRunAgent,
  useRegisterPushToken,
  useProgress,
  useAnalyzeProgress,
  useChatHistory,
  useSendChat,
  useClearChat,
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
  TodayBriefing,
  RunResult,
  PushRegisterRequest,
  ProgressReport,
  ProgressRead,
  ChatMessage,
  ChatHistory,
} from '../types';

const mockApiGet = apiGet as jest.MockedFunction<typeof apiGet>;
const mockApiPost = apiPost as jest.MockedFunction<typeof apiPost>;
const mockApiPut = apiPut as jest.MockedFunction<typeof apiPut>;
const mockApiUpload = apiUpload as jest.MockedFunction<typeof apiUpload>;
const mockApiDelete = apiDelete as jest.MockedFunction<typeof apiDelete>;

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
      run_constraints_json: '{}', goal_text: 'Build cardio',
      daily_run_time: '05:30', timezone: 'UTC', agent_enabled: true,
      updated_at: '2026-06-20T08:00:00Z',
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
      daily_run_time: '05:30', timezone: 'UTC', agent_enabled: true,
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
  it('uploads the image WITH the week_start field and returns the parsed week', async () => {
    const week: CrossFitWeek = { week_start: '2026-06-22', days: [] };
    mockApiUpload.mockResolvedValue(week);
    const { result } = await renderHook(() => useParseCrossfit(), { wrapper: createWrapper() });
    await act(async () => {
      result.current.mutate({
        file: { uri: 'file:///x.jpg', name: 'x.jpg', type: 'image/jpeg' },
        weekStart: '2026-06-22',
      });
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockApiUpload).toHaveBeenCalledWith(
      '/api/crossfit/parse',
      { uri: 'file:///x.jpg', name: 'x.jpg', type: 'image/jpeg' },
      { fields: { week_start: '2026-06-22' } },
    );
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

const todayBriefing: TodayBriefing = {
  date: '2026-06-20',
  readiness_color: 'amber',
  drivers: {
    date: '2026-06-20',
    sleep_hours: 6.1, sleep_score: 62,
    hrv_last_night_ms: 48, hrv_baseline_ms: 58.4, hrv_delta_pct: -17.8,
    rhr_last_night: 54, rhr_baseline: 50.2, rhr_delta_bpm: 3.8,
    body_battery_high: 61, recovery_trend: 'declining', data_complete: true,
  },
  reasons: ['HRV -17.8% vs baseline'],
  action: 'SOFTEN',
  original_session: null,
  effective_session: null,
  rationale: 'Trimmed.',
  source: 'ai',
  stale: false,
};

describe('useToday', () => {
  it('fetches /api/today', async () => {
    mockApiGet.mockResolvedValue(todayBriefing);
    const { result } = await renderHook(() => useToday(), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockApiGet).toHaveBeenCalledWith('/api/today');
    expect(result.current.data).toEqual(todayBriefing);
  });
});

describe('useUndoToday', () => {
  it('POSTs /api/today/undo and returns the reverted briefing', async () => {
    const reverted: TodayBriefing = { ...todayBriefing, action: 'STAND' };
    mockApiPost.mockResolvedValue(reverted);
    const { result } = await renderHook(() => useUndoToday(), { wrapper: createWrapper() });
    await act(async () => { result.current.mutate(); });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockApiPost).toHaveBeenCalledWith('/api/today/undo');
    expect(result.current.data).toEqual(reverted);
  });
});

describe('useRunAgent', () => {
  it('POSTs /api/agent/run and returns the run result', async () => {
    const runResult: RunResult = {
      date: '2026-06-20', skipped: false, readiness_color: 'amber',
      action: 'SOFTEN', source: 'ai', stale: false, pushed: true, error: null,
    };
    mockApiPost.mockResolvedValue(runResult);
    const { result } = await renderHook(() => useRunAgent(), { wrapper: createWrapper() });
    await act(async () => { result.current.mutate(); });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockApiPost).toHaveBeenCalledWith('/api/agent/run');
    expect(result.current.data).toEqual(runResult);
  });
});

describe('useRegisterPushToken', () => {
  it('POSTs /api/push/register with the token + platform body', async () => {
    const body: PushRegisterRequest = {
      expo_push_token: 'ExponentPushToken[abc]', platform: 'ios',
    };
    mockApiPost.mockResolvedValue(body);
    const { result } = await renderHook(() => useRegisterPushToken(), { wrapper: createWrapper() });
    await act(async () => { result.current.mutate(body); });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockApiPost).toHaveBeenCalledWith('/api/push/register', body);
  });
});

describe('useProgress', () => {
  it('fetches /api/progress with default weeks 12', async () => {
    const data: ProgressReport = {
      weeks: 12,
      generated_at: '2026-06-21T07:00:00Z',
      enough_data: true,
      signals: [],
    };
    mockApiGet.mockResolvedValue(data);
    const { result } = await renderHook(() => useProgress(), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockApiGet).toHaveBeenCalledWith('/api/progress?weeks=12');
    expect(result.current.data).toEqual(data);
  });

  it('fetches /api/progress with an explicit weeks value', async () => {
    mockApiGet.mockResolvedValue({
      weeks: 8, generated_at: '2026-06-21T07:00:00Z', enough_data: false, signals: [],
    });
    const { result } = await renderHook(() => useProgress(8), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockApiGet).toHaveBeenCalledWith('/api/progress?weeks=8');
  });
});

describe('useAnalyzeProgress', () => {
  it('POSTs /api/progress/analyze with the weeks body and returns the read', async () => {
    const read: ProgressRead = { text: 'Your engine is improving.', source: 'ai' };
    mockApiPost.mockResolvedValue(read);
    const { result } = await renderHook(() => useAnalyzeProgress(), { wrapper: createWrapper() });
    await act(async () => { result.current.mutate({ weeks: 12 }); });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockApiPost).toHaveBeenCalledWith('/api/progress/analyze', { weeks: 12 });
    expect(result.current.data).toEqual(read);
  });
});

describe('useChatHistory', () => {
  it('GETs /api/chat with the default limit (50)', async () => {
    const data: ChatHistory = {
      messages: [{ role: 'user', content: 'hi', created_at: '2026-06-22T09:00:00Z' }],
    };
    mockApiGet.mockResolvedValue(data);
    const { result } = await renderHook(() => useChatHistory(), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockApiGet).toHaveBeenCalledWith('/api/chat?limit=50');
    expect(result.current.data).toEqual(data);
  });

  it('passes a custom limit', async () => {
    mockApiGet.mockResolvedValue({ messages: [] });
    const { result } = await renderHook(() => useChatHistory(10), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockApiGet).toHaveBeenCalledWith('/api/chat?limit=10');
  });
});

describe('useSendChat', () => {
  it('POSTs /api/chat with the message and returns the assistant turn', async () => {
    const answer: ChatMessage = {
      role: 'assistant', content: 'Your Z2 pace is improving.', created_at: '2026-06-22T09:14:00Z',
    };
    mockApiPost.mockResolvedValue(answer);
    const { result } = await renderHook(() => useSendChat(), { wrapper: createWrapper() });
    await act(async () => { result.current.mutate({ message: 'How is my pace?' }); });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockApiPost).toHaveBeenCalledWith('/api/chat', { message: 'How is my pace?' });
    expect(result.current.data).toEqual(answer);
  });
});

describe('useClearChat', () => {
  it('DELETEs /api/chat', async () => {
    mockApiDelete.mockResolvedValue(undefined);
    const { result } = await renderHook(() => useClearChat(), { wrapper: createWrapper() });
    await act(async () => { result.current.mutate(); });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockApiDelete).toHaveBeenCalledWith('/api/chat');
  });
});
