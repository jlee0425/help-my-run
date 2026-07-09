export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}

let onUnauthorized: (() => void) | null = null;

/** Registered by App: invalidates auth state so the router redirects to login. */
export function setOnUnauthorized(fn: (() => void) | null) {
  onUnauthorized = fn;
}

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    credentials: 'same-origin',
    headers: init?.body ? { 'Content-Type': 'application/json' } : undefined,
    ...init,
  });
  if (res.status === 401) {
    onUnauthorized?.();
    throw new ApiError(401, 'unauthorized');
  }
  if (!res.ok) {
    let msg = res.statusText || `HTTP ${res.status}`;
    try {
      const body = (await res.json()) as { error?: string };
      if (body.error) msg = body.error;
    } catch {
      /* non-JSON error body */
    }
    throw new ApiError(res.status, msg);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

export const apiGet = <T,>(p: string) => api<T>(p);
export const apiPost = <T,>(p: string, body?: unknown) =>
  api<T>(p, { method: 'POST', body: body !== undefined ? JSON.stringify(body) : undefined });
export const apiPut = <T,>(p: string, body: unknown) =>
  api<T>(p, { method: 'PUT', body: JSON.stringify(body) });
export const apiDelete = <T,>(p: string, body?: unknown) =>
  api<T>(p, { method: 'DELETE', body: body !== undefined ? JSON.stringify(body) : undefined });

/** multipart upload (CrossFit schedule photo) — no JSON content-type. */
export async function apiUpload<T>(path: string, form: FormData): Promise<T> {
  const res = await fetch(path, { method: 'POST', credentials: 'same-origin', body: form });
  if (res.status === 401) {
    onUnauthorized?.();
    throw new ApiError(401, 'unauthorized');
  }
  if (!res.ok) {
    let msg = res.statusText || `HTTP ${res.status}`;
    try {
      const body = (await res.json()) as { error?: string };
      if (body.error) msg = body.error;
    } catch {
      /* non-JSON */
    }
    throw new ApiError(res.status, msg);
  }
  return (await res.json()) as T;
}
