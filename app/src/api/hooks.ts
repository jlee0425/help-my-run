import {
  useQuery,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query';
import { apiGet, apiPost, apiPut, apiDelete, apiUpload } from './client';
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
  PushRegisterResponse,
  ProgressReport,
  ProgressRead,
  StreamAnalysis,
  ChatMessage,
  ChatHistory,
} from './types';

export function useStatus() {
  return useQuery({
    queryKey: ['status'],
    queryFn: () => apiGet<Status>('/api/status'),
  });
}

export function useActivities(limit = 30) {
  return useQuery({
    queryKey: ['activities', limit],
    queryFn: () => apiGet<ActivitiesResponse>(`/api/activities?limit=${limit}`),
  });
}

export function useRecovery(days = 30) {
  return useQuery({
    queryKey: ['recovery', days],
    queryFn: () => apiGet<RecoveryResponse>(`/api/recovery?days=${days}`),
  });
}

export function useSync() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => apiPost<SyncResponse>('/api/sync'),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['status'] });
      queryClient.invalidateQueries({ queryKey: ['activities'] });
      queryClient.invalidateQueries({ queryKey: ['recovery'] });
    },
  });
}

export function useProfile() {
  return useQuery({
    queryKey: ['profile'],
    queryFn: () => apiGet<AthleteProfile>('/api/profile'),
  });
}

export function useUpdateProfile() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (profile: AthleteProfile) => apiPut<AthleteProfile>('/api/profile', profile),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['profile'] });
    },
  });
}

export function useFitness() {
  return useQuery({
    queryKey: ['fitness'],
    queryFn: () => apiGet<Fitness>('/api/fitness'),
  });
}

export function usePlan(week: string) {
  return useQuery({
    queryKey: ['plan', week],
    queryFn: () => apiGet<Plan>(`/api/plan?week=${week}`),
    enabled: !!week,
  });
}

export function useParseCrossfit() {
  return useMutation({
    mutationFn: ({
      file,
      weekStart,
    }: {
      file: { uri: string; name: string; type: string };
      weekStart: string;
    }) =>
      apiUpload<CrossFitWeek>('/api/crossfit/parse', file, {
        fields: { week_start: weekStart },
      }),
  });
}

export function useGeneratePlan() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: { week_start: string; crossfit_week?: CrossFitWeek }) =>
      apiPost<Plan>('/api/plan/generate', body),
    onSuccess: (plan) => {
      queryClient.invalidateQueries({ queryKey: ['plan', plan.week_start] });
      queryClient.invalidateQueries({ queryKey: ['fitness'] });
    },
  });
}

export function useToday() {
  return useQuery({
    queryKey: ['today'],
    queryFn: () => apiGet<TodayBriefing>('/api/today'),
  });
}

export function useUndoToday() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => apiPost<TodayBriefing>('/api/today/undo'),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['today'] });
    },
  });
}

export function useRunAgent() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => apiPost<RunResult>('/api/agent/run'),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['today'] });
    },
  });
}

export function useRegisterPushToken() {
  return useMutation({
    mutationFn: (body: PushRegisterRequest) =>
      apiPost<PushRegisterResponse>('/api/push/register', body),
  });
}

export function useProgress(weeks = 12) {
  return useQuery({
    queryKey: ['progress', weeks],
    queryFn: () => apiGet<ProgressReport>(`/api/progress?weeks=${weeks}`),
  });
}

export function useAnalyzeProgress() {
  return useMutation({
    mutationFn: (body: { weeks?: number }) =>
      apiPost<ProgressRead>('/api/progress/analyze', body),
  });
}

export function useActivityAnalysis(activityId: number) {
  return useQuery({
    queryKey: ['analysis', activityId],
    queryFn: () => apiGet<StreamAnalysis>(`/api/activities/${activityId}/analysis`),
    enabled: Number.isFinite(activityId),
    // GET returns 200 + { has_stream:false } when not fetched, so no 404 branch needed.
  });
}

export function useFetchStream(activityId: number) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => apiPost<StreamAnalysis>(`/api/activities/${activityId}/stream/fetch`),
    onSuccess: (data) => {
      queryClient.setQueryData(['analysis', activityId], data);
      queryClient.invalidateQueries({ queryKey: ['progress'] });
    },
  });
}

export function useChatHistory(limit = 50) {
  return useQuery({
    queryKey: ['chat'],
    queryFn: () => apiGet<ChatHistory>(`/api/chat?limit=${limit}`),
  });
}

export function useSendChat() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: { message: string }) =>
      apiPost<ChatMessage>('/api/chat', body),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['chat'] });
    },
  });
}

export function useClearChat() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => apiDelete<void>('/api/chat'),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['chat'] });
    },
  });
}
