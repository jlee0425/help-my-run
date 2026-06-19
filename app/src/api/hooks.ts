import {
  useQuery,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query';
import * as WebBrowser from 'expo-web-browser';
import { apiGet, apiPost } from './client';
import type {
  Status,
  ActivitiesResponse,
  RecoveryResponse,
  SyncResponse,
  ConnectResponse,
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

export function useConnectStrava() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async () => {
      const { authorizeUrl } = await apiGet<ConnectResponse>('/api/strava/connect');
      await WebBrowser.openAuthSessionAsync(authorizeUrl);
      for (let i = 0; i < 30; i++) {
        const s = await apiGet<Status>('/api/status');
        if (s.strava?.connected) return s;
        await new Promise((r) => setTimeout(r, 2000));
      }
      return apiGet<Status>('/api/status');
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['status'] });
    },
  });
}
