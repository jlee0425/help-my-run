import { apiGet, apiPost } from './client';

export type AuthState = { setup_required: boolean; authed: boolean; demo: boolean };

export const authState = () => apiGet<AuthState>('/api/auth/state');
export const setup = (password: string) =>
  apiPost<{ api_token: string }>('/api/setup', { password });
export const login = (password: string) => apiPost<void>('/api/login', { password });
export const logout = () => apiPost<void>('/api/logout');
export const changePassword = (current: string, next: string) =>
  apiPost<void>('/api/auth/password', { current, new: next });
export const regenerateToken = () => apiPost<{ api_token: string }>('/api/auth/token');
