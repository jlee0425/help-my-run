import { afterEach, describe, expect, it, vi } from 'vitest';
import { subscribePush, urlBase64ToUint8Array } from './push';

afterEach(() => vi.unstubAllGlobals());

describe('urlBase64ToUint8Array', () => {
  it('decodes base64url with padding restoration', () => {
    // "hello" -> aGVsbG8 (base64url, unpadded)
    expect(Array.from(urlBase64ToUint8Array('aGVsbG8'))).toEqual([104, 101, 108, 108, 111]);
    // url-safe chars - and _ map to + and /  ( 0xfb 0xef -> "----" is invalid; use realistic pair )
    expect(Array.from(urlBase64ToUint8Array('_-8'))).toEqual([255, 239]);
  });
});

describe('subscribePush', () => {
  it('fetches the VAPID key, subscribes, and posts the subscription', async () => {
    const subscribe = vi.fn(async () => ({
      endpoint: 'https://push.example/ep1',
      toJSON: () => ({ keys: { p256dh: 'pk', auth: 'ak' } }),
    }));
    vi.stubGlobal('Notification', { requestPermission: vi.fn(async () => 'granted') });
    Object.defineProperty(navigator, 'serviceWorker', {
      configurable: true,
      value: {
        ready: Promise.resolve({ pushManager: { subscribe } }),
        getRegistration: async () => undefined,
      },
    });
    vi.stubGlobal('PushManager', function PushManager() {});

    const fetchCalls: Array<{ path: string; init?: RequestInit }> = [];
    vi.stubGlobal(
      'fetch',
      vi.fn(async (path: string, init?: RequestInit) => {
        fetchCalls.push({ path, init });
        const body = path.includes('vapid') ? { key: 'aGVsbG8' } : undefined;
        return {
          ok: true,
          status: path.includes('vapid') ? 200 : 204,
          statusText: 'ok',
          json: async () => body,
        } as Response;
      }),
    );

    await subscribePush();

    expect(subscribe).toHaveBeenCalledWith(
      expect.objectContaining({ userVisibleOnly: true }),
    );
    const post = fetchCalls.find((c) => c.init?.method === 'POST');
    expect(post?.path).toBe('/api/push/subscribe');
    expect(JSON.parse(String(post?.init?.body))).toEqual({
      endpoint: 'https://push.example/ep1',
      keys: { p256dh: 'pk', auth: 'ak' },
    });
  });

  it('throws when permission is denied', async () => {
    vi.stubGlobal('Notification', { requestPermission: vi.fn(async () => 'denied') });
    vi.stubGlobal('PushManager', function PushManager() {});
    Object.defineProperty(navigator, 'serviceWorker', {
      configurable: true,
      value: { ready: Promise.resolve({}), getRegistration: async () => undefined },
    });
    await expect(subscribePush()).rejects.toThrow(/permission/i);
  });
});
