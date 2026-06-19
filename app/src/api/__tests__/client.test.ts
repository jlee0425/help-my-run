import { apiGet, apiPost, ApiError } from '../client';

jest.mock('../config', () => ({
  getBaseUrl: jest.fn(),
  getToken: jest.fn(),
}));

import { getBaseUrl, getToken } from '../config';

const mockGetBaseUrl = getBaseUrl as jest.MockedFunction<typeof getBaseUrl>;
const mockGetToken = getToken as jest.MockedFunction<typeof getToken>;

function mockFetchOnce(opts: { ok: boolean; status: number; json?: unknown }) {
  (global.fetch as jest.Mock).mockResolvedValueOnce({
    ok: opts.ok,
    status: opts.status,
    json: async () => opts.json,
  });
}

beforeEach(() => {
  global.fetch = jest.fn() as jest.Mock;
  mockGetBaseUrl.mockResolvedValue('http://localhost:8080');
  mockGetToken.mockResolvedValue('test-token');
});

afterEach(() => {
  jest.clearAllMocks();
});

describe('apiGet', () => {
  it('prepends base URL and sends bearer + content-type headers', async () => {
    mockFetchOnce({ ok: true, status: 200, json: { status: 'ok' } });

    const data = await apiGet<{ status: string }>('/health');

    expect(global.fetch).toHaveBeenCalledTimes(1);
    const [url, init] = (global.fetch as jest.Mock).mock.calls[0];
    expect(url).toBe('http://localhost:8080/health');
    expect(init.headers).toMatchObject({
      'Content-Type': 'application/json',
      Authorization: 'Bearer test-token',
    });
    expect(data).toEqual({ status: 'ok' });
  });

  it('omits Authorization header when no token is stored', async () => {
    mockGetToken.mockResolvedValue(null);
    mockFetchOnce({ ok: true, status: 200, json: {} });

    await apiGet('/api/status');

    const [, init] = (global.fetch as jest.Mock).mock.calls[0];
    expect(init.headers.Authorization).toBeUndefined();
  });

  it('throws ApiError with the response status on non-ok response', async () => {
    mockFetchOnce({ ok: false, status: 401, json: { error: 'unauthorized' } });

    // Capture the rejection ONCE (only one mock is queued) and assert both
    // its shape and its instanceof on that single error.
    const err = await apiGet('/api/status').catch((e) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({ name: 'ApiError', status: 401 });
  });

  it('throws ApiError(0) when base URL is not configured', async () => {
    mockGetBaseUrl.mockResolvedValue(null);

    await expect(apiGet('/api/status')).rejects.toMatchObject({
      status: 0,
      message: 'Backend URL not configured',
    });
    expect(global.fetch).not.toHaveBeenCalled();
  });
});

describe('apiPost', () => {
  it('uses POST and serializes the body', async () => {
    mockFetchOnce({
      ok: true,
      status: 200,
      json: { strava: { status: 'ok', synced: 1, error: null } },
    });

    const data = await apiPost<{ strava: { status: string } }>('/api/sync', { foo: 1 });

    const [url, init] = (global.fetch as jest.Mock).mock.calls[0];
    expect(url).toBe('http://localhost:8080/api/sync');
    expect(init.method).toBe('POST');
    expect(init.body).toBe(JSON.stringify({ foo: 1 }));
    expect(data).toEqual({ strava: { status: 'ok', synced: 1, error: null } });
  });

  it('sends no body when none is provided', async () => {
    mockFetchOnce({ ok: true, status: 200, json: {} });

    await apiPost('/api/sync');

    const [, init] = (global.fetch as jest.Mock).mock.calls[0];
    expect(init.method).toBe('POST');
    expect(init.body).toBeUndefined();
  });

  it('returns undefined for a 204 No Content response', async () => {
    (global.fetch as jest.Mock).mockResolvedValueOnce({
      ok: true,
      status: 204,
      json: async () => {
        throw new Error('should not parse json on 204');
      },
    });

    const data = await apiPost('/api/sync');
    expect(data).toBeUndefined();
  });
});
