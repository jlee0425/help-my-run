import * as SecureStore from 'expo-secure-store';

const BASE_URL_KEY = 'hmr.baseUrl';
const TOKEN_KEY = 'hmr.token';

export async function saveConfig(baseUrl: string, token: string): Promise<void> {
  await SecureStore.setItemAsync(BASE_URL_KEY, baseUrl);
  await SecureStore.setItemAsync(TOKEN_KEY, token);
}

export async function getBaseUrl(): Promise<string | null> {
  return SecureStore.getItemAsync(BASE_URL_KEY);
}

export async function getToken(): Promise<string | null> {
  return SecureStore.getItemAsync(TOKEN_KEY);
}

export async function clearConfig(): Promise<void> {
  await SecureStore.deleteItemAsync(BASE_URL_KEY);
  await SecureStore.deleteItemAsync(TOKEN_KEY);
}
