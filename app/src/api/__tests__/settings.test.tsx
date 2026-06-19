import React from 'react';
import { renderHook, waitFor, act } from '@testing-library/react-native';

jest.mock('../config', () => ({
  getBaseUrl: jest.fn(),
  getToken: jest.fn(),
  saveConfig: jest.fn(),
  clearConfig: jest.fn(),
}));

import { getBaseUrl, getToken, saveConfig } from '../config';
import { useSettings } from '../settings';

const mockGetBaseUrl = getBaseUrl as jest.MockedFunction<typeof getBaseUrl>;
const mockGetToken = getToken as jest.MockedFunction<typeof getToken>;
const mockSaveConfig = saveConfig as jest.MockedFunction<typeof saveConfig>;

afterEach(() => {
  jest.clearAllMocks();
});

describe('useSettings', () => {
  it('loads stored baseUrl + token on mount', async () => {
    // Defer resolution so the loading flag's true state is observable before
    // the mount effect settles. (renderHook is async in this version of
    // @testing-library/react-native, so a plain mockResolvedValue would have
    // already flushed the effect by the time the awaited render resolves.)
    let resolveBaseUrl!: (v: string) => void;
    let resolveToken!: (v: string) => void;
    mockGetBaseUrl.mockReturnValue(
      new Promise<string>((resolve) => {
        resolveBaseUrl = resolve;
      }),
    );
    mockGetToken.mockReturnValue(
      new Promise<string>((resolve) => {
        resolveToken = resolve;
      }),
    );

    const { result } = await renderHook(() => useSettings());

    expect(result.current.loading).toBe(true);

    await act(async () => {
      resolveBaseUrl('http://localhost:8080');
      resolveToken('stored-token');
    });

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.baseUrl).toBe('http://localhost:8080');
    expect(result.current.token).toBe('stored-token');
  });

  it('defaults to empty strings when nothing is stored', async () => {
    mockGetBaseUrl.mockResolvedValue(null);
    mockGetToken.mockResolvedValue(null);

    const { result } = await renderHook(() => useSettings());

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.baseUrl).toBe('');
    expect(result.current.token).toBe('');
  });

  it('save persists via saveConfig and updates state', async () => {
    mockGetBaseUrl.mockResolvedValue(null);
    mockGetToken.mockResolvedValue(null);
    mockSaveConfig.mockResolvedValue(undefined);

    const { result } = await renderHook(() => useSettings());
    await waitFor(() => expect(result.current.loading).toBe(false));

    await act(async () => {
      await result.current.save('http://10.0.0.5:8080', 'new-token');
    });

    expect(mockSaveConfig).toHaveBeenCalledWith('http://10.0.0.5:8080', 'new-token');
    expect(result.current.baseUrl).toBe('http://10.0.0.5:8080');
    expect(result.current.token).toBe('new-token');
  });
});
