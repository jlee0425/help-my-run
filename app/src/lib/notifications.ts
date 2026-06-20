import * as Notifications from 'expo-notifications';
import Constants from 'expo-constants';
import { Platform } from 'react-native';
import { apiPost } from '../api/client';
import type { PushRegisterRequest } from '../api/types';

// Module-scope handler: how to present a notification received while the app is
// foregrounded. SDK 56 uses shouldShowBanner/shouldShowList (shouldShowAlert removed).
Notifications.setNotificationHandler({
  handleNotification: async () => ({
    shouldShowBanner: true,
    shouldShowList: true,
    shouldPlaySound: true,
    shouldSetBadge: false,
  }),
});

function getProjectId(): string | null {
  const fromExpoConfig = (Constants?.expoConfig as { extra?: { eas?: { projectId?: string } } } | undefined)
    ?.extra?.eas?.projectId;
  const fromEasConfig = (Constants?.easConfig as { projectId?: string } | undefined)?.projectId;
  return fromExpoConfig ?? fromEasConfig ?? null;
}

/**
 * Requests notification permission, obtains the Expo push token, and POSTs it to
 * the backend. Returns the token string on success, or null on simulator /
 * denied permission / missing projectId (push needs a dev build, not Expo Go).
 */
export async function registerForPushNotificationsAsync(): Promise<string | null> {
  if (Platform.OS === 'android') {
    await Notifications.setNotificationChannelAsync('default', {
      name: 'default',
      importance: Notifications.AndroidImportance.MAX,
      vibrationPattern: [0, 250, 250, 250],
      lightColor: '#FF231F7C',
    });
  }

  const { status: existingStatus } = await Notifications.getPermissionsAsync();
  let finalStatus = existingStatus;
  if (existingStatus !== 'granted') {
    const { status } = await Notifications.requestPermissionsAsync();
    finalStatus = status;
  }
  if (finalStatus !== 'granted') return null;

  const projectId = getProjectId();
  if (!projectId) return null;

  const tokenResp = await Notifications.getExpoPushTokenAsync({ projectId });
  const token = tokenResp.data;

  const body: PushRegisterRequest = {
    expo_push_token: token,
    platform: Platform.OS === 'android' ? 'android' : 'ios',
  };
  await apiPost('/api/push/register', body);

  return token;
}
