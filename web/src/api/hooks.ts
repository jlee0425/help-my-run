// Query/mutation hooks. Extended per-screen as tasks land (M5 plan T6).
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { authState, login, logout, setup } from './auth';
import { apiGet } from './client';
import type { Status } from './types';

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

export function useStatus() {
  return useQuery({ queryKey: ['status'], queryFn: () => apiGet<Status>('/api/status') });
}
