// Query/mutation hooks over the Go API (wire shapes in ./types.ts).
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { authState, login, logout, setup } from './auth';
import { api, ApiError, apiDelete, apiGet, apiPost, apiPut, apiUpload } from './client';
import type {
  Activity,
  AgentRunResult,
  ChatHistory,
  ChatMessage,
  CrossFitWeek,
  FitnessMetrics,
  Plan,
  Profile,
  ProgressReport,
  RecoveryDay,
  Status,
  StreamAnalysis,
  Today,
} from './types';

export { useToday } from './today';

// --- auth -------------------------------------------------------------

export function useAuthState() {
  return useQuery({ queryKey: ['auth'], queryFn: authState, staleTime: 30_000 });
}

export function useLogin() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (password: string) => login(password),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['auth'] }),
  });
}

export function useSetup() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (password: string) => setup(password),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['auth'] }),
  });
}

export function useLogout() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => logout(),
    onSuccess: () => qc.invalidateQueries(),
  });
}

// --- core reads --------------------------------------------------------

export function useStatus() {
  return useQuery({ queryKey: ['status'], queryFn: () => apiGet<Status>('/api/status') });
}

export function useRecovery(days: number) {
  return useQuery({
    queryKey: ['recovery', days],
    queryFn: async () =>
      (await apiGet<{ recovery: RecoveryDay[] }>(`/api/recovery?days=${days}`)).recovery,
  });
}

export function useActivities(limit = 200) {
  return useQuery({
    queryKey: ['activities', limit],
    queryFn: async () =>
      (await apiGet<{ activities: Activity[] }>(`/api/activities?limit=${limit}`)).activities,
  });
}

export function useProgress(weeks = 12) {
  return useQuery({
    queryKey: ['progress', weeks],
    queryFn: () => apiGet<ProgressReport>(`/api/progress?weeks=${weeks}`),
  });
}

export function useAnalysis(activityId: number | undefined) {
  return useQuery({
    queryKey: ['analysis', activityId],
    enabled: activityId !== undefined,
    queryFn: () => apiGet<StreamAnalysis>(`/api/activities/${activityId}/analysis`),
  });
}

export function useFetchStream() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (activityId: number) =>
      apiPost<StreamAnalysis>(`/api/activities/${activityId}/stream/fetch`),
    onSuccess: (_data, activityId) =>
      qc.invalidateQueries({ queryKey: ['analysis', activityId] }),
  });
}

// --- sync + agent -------------------------------------------------------

export function useSync() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => apiPost<{ garmin: { status: string; synced: number } }>('/api/sync'),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['status'] });
      void qc.invalidateQueries({ queryKey: ['activities'] });
      void qc.invalidateQueries({ queryKey: ['recovery'] });
    },
  });
}

export function useAgentRun() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (opts?: { force?: boolean }) =>
      apiPost<AgentRunResult>(`/api/agent/run${opts?.force ? '?force=true' : ''}`),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ['today'] }),
  });
}

export function useUndoToday() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => apiPost<Today>('/api/today/undo'),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ['today'] }),
  });
}

// --- chat ----------------------------------------------------------------

export function useChatHistory() {
  return useQuery({
    queryKey: ['chat'],
    queryFn: async () => (await apiGet<ChatHistory>('/api/chat?limit=100')).messages,
  });
}

export function useSendChat() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (message: string) => apiPost<ChatMessage>('/api/chat', { message }),
    onSettled: () => void qc.invalidateQueries({ queryKey: ['chat'] }),
  });
}

export function useClearChat() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => apiDelete<void>('/api/chat'),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ['chat'] }),
  });
}

// --- plan ------------------------------------------------------------------

export function usePlan(weekStart: string) {
  return useQuery<Plan | null>({
    queryKey: ['plan', weekStart],
    queryFn: async () => {
      try {
        return await api<Plan>(`/api/plan?week=${weekStart}`);
      } catch (e) {
        if (e instanceof ApiError && e.status === 404) return null;
        throw e;
      }
    },
  });
}

export function useCrossfitParse() {
  return useMutation({
    mutationFn: ({ weekStart, image }: { weekStart: string; image: File }) => {
      const form = new FormData();
      form.append('week_start', weekStart);
      form.append('image', image);
      return apiUpload<CrossFitWeek>('/api/crossfit/parse', form);
    },
  });
}

export function usePlanGenerate() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ weekStart, crossfitWeek }: { weekStart: string; crossfitWeek?: CrossFitWeek }) =>
      apiPost<Plan>('/api/plan/generate', {
        week_start: weekStart,
        crossfit_week: crossfitWeek,
      }),
    onSuccess: (_p, vars) => void qc.invalidateQueries({ queryKey: ['plan', vars.weekStart] }),
  });
}

export function useFitness() {
  return useQuery({
    queryKey: ['fitness'],
    queryFn: () => apiGet<FitnessMetrics>('/api/fitness'),
  });
}

// --- profile -----------------------------------------------------------------

export function useProfile() {
  return useQuery<Profile | null>({
    queryKey: ['profile'],
    queryFn: async () => {
      try {
        return await api<Profile>('/api/profile');
      } catch (e) {
        if (e instanceof ApiError && e.status === 404) return null; // fresh instance
        throw e;
      }
    },
  });
}

export function useUpdateProfile() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (p: Partial<Profile>) => apiPut<Profile>('/api/profile', p),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['profile'] });
      void qc.invalidateQueries({ queryKey: ['status'] }); // agent schedule may change
    },
  });
}
