import { afterEach, describe, expect, it, vi } from 'vitest';
import { api, ApiError, apiPost, setOnUnauthorized } from './client';

function mockFetch(status: number, body?: unknown) {
  const res = {
    ok: status >= 200 && status < 300,
    status,
    statusText: `HTTP ${status}`,
    json: async () => body,
  } as Response;
  const fn = vi.fn(async () => res);
  vi.stubGlobal('fetch', fn);
  return fn;
}

afterEach(() => {
  setOnUnauthorized(null);
  vi.unstubAllGlobals();
});

describe('api client', () => {
  it('returns parsed JSON on 200', async () => {
    mockFetch(200, { hello: 'world' });
    await expect(api<{ hello: string }>('/api/x')).resolves.toEqual({ hello: 'world' });
  });

  it('extracts {error} message from failed responses', async () => {
    mockFetch(502, { error: 'Claude not logged in — run `claude auth login`.' });
    const err = await api('/api/chat').catch((e) => e as ApiError);
    expect(err).toBeInstanceOf(ApiError);
    expect((err as ApiError).status).toBe(502);
    expect((err as ApiError).message).toContain('claude auth login');
  });

  it('fires the unauthorized handler on 401', async () => {
    mockFetch(401, { error: 'unauthorized' });
    const onUnauth = vi.fn();
    setOnUnauthorized(onUnauth);
    await expect(api('/api/status')).rejects.toBeInstanceOf(ApiError);
    expect(onUnauth).toHaveBeenCalledOnce();
  });

  it('resolves undefined on 204 and sends JSON bodies', async () => {
    const fn = mockFetch(204);
    await expect(apiPost('/api/login', { password: 'x' })).resolves.toBeUndefined();
    const [, init] = fn.mock.calls[0] as unknown as [string, RequestInit];
    expect(init.method).toBe('POST');
    expect(init.body).toBe(JSON.stringify({ password: 'x' }));
    expect((init.headers as Record<string, string>)['Content-Type']).toBe('application/json');
  });
});
