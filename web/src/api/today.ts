import { useQuery } from '@tanstack/react-query';
import { api, ApiError } from './client';
import type { Today } from './types';

/** GET /api/today — 404 (no decision yet) resolves to null, not an error. */
export function useToday(date?: string) {
  return useQuery<Today | null>({
    queryKey: ['today', date ?? 'auto'],
    queryFn: async () => {
      try {
        return await api<Today>(date ? `/api/today?date=${date}` : '/api/today');
      } catch (e) {
        if (e instanceof ApiError && e.status === 404) return null;
        throw e;
      }
    },
  });
}
