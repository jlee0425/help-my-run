import React from 'react';
import { renderHook, waitFor, act } from '@testing-library/react-native';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { StreamAnalysis } from '../types';

const mockApiGet = jest.fn();
const mockApiPost = jest.fn();
jest.mock('../client', () => ({
  apiGet: (...a: unknown[]) => mockApiGet(...a),
  apiPost: (...a: unknown[]) => mockApiPost(...a),
  apiPut: jest.fn(),
  apiUpload: jest.fn(),
}));

import { useActivityAnalysis, useFetchStream } from '../hooks';

const analysis: StreamAnalysis = {
  activity_id: 14820001234, has_stream: true, has_hr: true,
  time_in_zone: [{ zone: 2, seconds: 1800, pct: 100 }],
  decoupling_pct: 4.2, pa_hr_first: 0.0212, pa_hr_second: 0.0203,
  zones: { z1_hi: 116, z2_hi: 145, z3_hi: 157.5, z4_hi: 170 },
  source: 'strava', computed_at: '2026-06-22T07:00:00Z',
};

function wrapper(client: QueryClient) {
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
}

afterEach(() => jest.clearAllMocks());

describe('useActivityAnalysis', () => {
  it('GETs /api/activities/{id}/analysis and returns the analysis', async () => {
    mockApiGet.mockResolvedValueOnce(analysis);
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const { result } = await renderHook(() => useActivityAnalysis(14820001234), { wrapper: wrapper(client) });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockApiGet).toHaveBeenCalledWith('/api/activities/14820001234/analysis');
    expect(result.current.data?.has_stream).toBe(true);
  });

  it('is disabled for a non-finite id (NaN)', async () => {
    const client = new QueryClient();
    const { result } = await renderHook(() => useActivityAnalysis(Number.NaN), { wrapper: wrapper(client) });
    expect(result.current.fetchStatus).toBe('idle');
    expect(mockApiGet).not.toHaveBeenCalled();
  });
});

describe('useFetchStream', () => {
  it('POSTs /stream/fetch and seeds the analysis cache on success', async () => {
    mockApiPost.mockResolvedValueOnce(analysis);
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const { result } = await renderHook(() => useFetchStream(14820001234), { wrapper: wrapper(client) });
    await act(async () => { await result.current.mutateAsync(); });
    expect(mockApiPost).toHaveBeenCalledWith('/api/activities/14820001234/stream/fetch');
    expect(client.getQueryData(['analysis', 14820001234])).toEqual(analysis);
  });
});
