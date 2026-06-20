import { getBaseUrl, getToken } from './config';

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
  }
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const baseUrl = await getBaseUrl();
  const token = await getToken();
  if (!baseUrl) throw new ApiError(0, 'Backend URL not configured');

  const res = await fetch(`${baseUrl}${path}`, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...init.headers,
    },
  });

  if (!res.ok) {
    throw new ApiError(res.status, `${init.method ?? 'GET'} ${path} failed: ${res.status}`);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

export const apiGet = <T>(path: string) => request<T>(path);
export const apiPost = <T>(path: string, body?: unknown) =>
  request<T>(path, { method: 'POST', body: body ? JSON.stringify(body) : undefined });

export const apiPut = <T>(path: string, body?: unknown) =>
  request<T>(path, { method: 'PUT', body: body ? JSON.stringify(body) : undefined });

export async function apiUpload<T>(
  path: string,
  file: { uri: string; name: string; type: string },
  field = 'image',
): Promise<T> {
  const baseUrl = await getBaseUrl();
  const token = await getToken();
  if (!baseUrl) throw new ApiError(0, 'Backend URL not configured');

  const form = new FormData();
  // RN FormData accepts this object shape; cast to satisfy DOM lib types.
  form.append(field, { uri: file.uri, name: file.name, type: file.type } as unknown as Blob);

  const res = await fetch(`${baseUrl}${path}`, {
    method: 'POST',
    headers: {
      // No Content-Type: RN sets multipart/form-data + boundary automatically.
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: form,
  });

  if (!res.ok) {
    throw new ApiError(res.status, `POST ${path} failed: ${res.status}`);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}
