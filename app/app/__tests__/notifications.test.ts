import { Platform } from 'react-native';

const mockSetChannel = jest.fn();
const mockGetPerms = jest.fn();
const mockReqPerms = jest.fn();
const mockGetToken = jest.fn();

jest.mock('expo-notifications', () => ({
  setNotificationChannelAsync: (...a: unknown[]) => mockSetChannel(...a),
  getPermissionsAsync: () => mockGetPerms(),
  requestPermissionsAsync: () => mockReqPerms(),
  getExpoPushTokenAsync: (...a: unknown[]) => mockGetToken(...a),
  // setNotificationHandler runs at module-eval (import time) before the file's
  // `const` mocks initialize, so it must be a self-contained fn (no TDZ ref).
  setNotificationHandler: jest.fn(),
  AndroidImportance: { MAX: 5 },
}));

jest.mock('expo-constants', () => ({
  __esModule: true,
  default: { expoConfig: { extra: { eas: { projectId: 'proj-123' } } }, easConfig: undefined },
}));

const mockApiPost = jest.fn();
jest.mock('../../src/api/client', () => ({
  apiPost: (...a: unknown[]) => mockApiPost(...a),
}));

import { registerForPushNotificationsAsync } from '../../src/lib/notifications';

afterEach(() => {
  jest.clearAllMocks();
  Platform.OS = 'ios';
});

describe('registerForPushNotificationsAsync', () => {
  it('returns null and does not POST when permission is denied', async () => {
    mockGetPerms.mockResolvedValue({ status: 'denied' });
    mockReqPerms.mockResolvedValue({ status: 'denied' });

    const token = await registerForPushNotificationsAsync();

    expect(token).toBeNull();
    expect(mockGetToken).not.toHaveBeenCalled();
    expect(mockApiPost).not.toHaveBeenCalled();
  });

  it('requests permission only when not already granted', async () => {
    mockGetPerms.mockResolvedValue({ status: 'granted' });
    mockGetToken.mockResolvedValue({ data: 'ExponentPushToken[abc]', type: 'expo' });
    mockApiPost.mockResolvedValue({});

    await registerForPushNotificationsAsync();

    expect(mockReqPerms).not.toHaveBeenCalled();
  });

  it('gets the token with the projectId and POSTs it (ios platform)', async () => {
    mockGetPerms.mockResolvedValue({ status: 'granted' });
    mockGetToken.mockResolvedValue({ data: 'ExponentPushToken[abc]', type: 'expo' });
    mockApiPost.mockResolvedValue({});

    const token = await registerForPushNotificationsAsync();

    expect(mockGetToken).toHaveBeenCalledWith({ projectId: 'proj-123' });
    expect(token).toBe('ExponentPushToken[abc]');
    expect(mockApiPost).toHaveBeenCalledWith('/api/push/register', {
      expo_push_token: 'ExponentPushToken[abc]',
      platform: 'ios',
    });
  });

  it('creates the android channel BEFORE requesting permission on android', async () => {
    Platform.OS = 'android';
    const order: string[] = [];
    mockSetChannel.mockImplementation(async () => { order.push('channel'); });
    mockGetPerms.mockImplementation(async () => { order.push('getPerms'); return { status: 'denied' }; });
    mockReqPerms.mockImplementation(async () => { order.push('reqPerms'); return { status: 'denied' }; });

    await registerForPushNotificationsAsync();

    expect(mockSetChannel).toHaveBeenCalledWith('default', expect.objectContaining({ name: 'default' }));
    expect(order[0]).toBe('channel');
    expect(order.indexOf('channel')).toBeLessThan(order.indexOf('getPerms'));
  });

  it('returns null (no throw) when projectId is missing', async () => {
    jest.resetModules();
    jest.doMock('expo-constants', () => ({
      __esModule: true,
      default: { expoConfig: { extra: {} }, easConfig: undefined },
    }));
    jest.doMock('expo-notifications', () => ({
      setNotificationChannelAsync: jest.fn(),
      getPermissionsAsync: () => Promise.resolve({ status: 'granted' }),
      requestPermissionsAsync: jest.fn(),
      getExpoPushTokenAsync: jest.fn(),
      setNotificationHandler: jest.fn(),
      AndroidImportance: { MAX: 5 },
    }));
    jest.doMock('../../src/api/client', () => ({ apiPost: jest.fn() }));
    const mod = require('../../src/lib/notifications');
    const token = await mod.registerForPushNotificationsAsync();
    expect(token).toBeNull();
  });
});
