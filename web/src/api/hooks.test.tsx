import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderHook, waitFor } from '@testing-library/react';
import type { ReactNode } from 'react';
import { describe, expect, it, vi } from 'vitest';

const calls: Array<{ path: string; init?: RequestInit }> = [];

function mockFetchRouter(routes: Record<string, { status?: number; body: unknown }>) {
  calls.length = 0;
  vi.stubGlobal(
    'fetch',
    vi.fn(async (path: string, init?: RequestInit) => {
      calls.push({ path, init });
      const hit = Object.entries(routes).find(([k]) => path.startsWith(k));
      const { status = 200, body } = hit ? hit[1] : { status: 404, body: { error: 'not found' } };
      return {
        ok: status >= 200 && status < 300,
        status,
        statusText: `HTTP ${status}`,
        json: async () => body,
      } as Response;
    }),
  );
}

function wrapper({ children }: { children: ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: 0 }, mutations: { retry: 0 } } });
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
}

describe('api hooks', () => {
  it('useRecovery hits /api/recovery?days=N and unwraps the list', async () => {
    const { useRecovery } = await import('./hooks');
    mockFetchRouter({ '/api/recovery?days=7': { body: { recovery: [{ date: '2026-07-08' }] } } });
    const { result } = renderHook(() => useRecovery(7), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual([{ date: '2026-07-08' }]);
    expect(calls[0].path).toBe('/api/recovery?days=7');
  });

  it('useToday resolves null on 404 (no decision yet)', async () => {
    const { useToday } = await import('./hooks');
    mockFetchRouter({ '/api/today': { status: 404, body: { error: 'no decision for date' } } });
    const { result } = renderHook(() => useToday(), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toBeNull();
  });

  it('usePlan passes ?week= and resolves null on 404', async () => {
    const { usePlan } = await import('./hooks');
    mockFetchRouter({ '/api/plan?week=2026-07-06': { status: 404, body: { error: 'no plan' } } });
    const { result } = renderHook(() => usePlan('2026-07-06'), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toBeNull();
    expect(calls[0].path).toBe('/api/plan?week=2026-07-06');
  });

  it('useSendChat posts the message and invalidates chat history', async () => {
    const { useSendChat, useChatHistory } = await import('./hooks');
    mockFetchRouter({
      '/api/chat?limit=100': { body: { messages: [] } },
      '/api/chat': { body: { role: 'assistant', content: 'hi', created_at: 'x' } },
    });
    const qc = new QueryClient({
      defaultOptions: { queries: { retry: 0 }, mutations: { retry: 0 } },
    });
    const wrap = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={qc}>{children}</QueryClientProvider>
    );
    const hist = renderHook(() => useChatHistory(), { wrapper: wrap });
    await waitFor(() => expect(hist.result.current.isSuccess).toBe(true));

    const send = renderHook(() => useSendChat(), { wrapper: wrap });
    send.result.current.mutate('Why easy today?');
    await waitFor(() => expect(send.result.current.isSuccess).toBe(true));
    const postCall = calls.find((c) => c.init?.method === 'POST');
    expect(postCall?.path).toBe('/api/chat');
    expect(postCall?.init?.body).toBe(JSON.stringify({ message: 'Why easy today?' }));
  });
});
