/// <reference lib="webworker" />
// Custom service worker (vite-plugin-pwa injectManifest): precache the app
// shell, network-only for /api (coach data must never be stale), offline
// fallback for navigations, and the Web Push notification handlers.
import { precacheAndRoute } from 'workbox-precaching';

declare const self: ServiceWorkerGlobalScope;

precacheAndRoute(self.__WB_MANIFEST);

self.addEventListener('install', () => {
  void self.skipWaiting();
});

self.addEventListener('activate', (e) => {
  e.waitUntil(self.clients.claim());
});

self.addEventListener('fetch', (e) => {
  const url = new URL(e.request.url);
  if (url.pathname.startsWith('/api/') || url.pathname === '/health') return; // network-only
  if (e.request.mode === 'navigate') {
    e.respondWith(
      fetch(e.request).catch(async () => {
        const cached = await caches.match('/offline.html');
        return cached ?? new Response('Offline', { status: 503 });
      }),
    );
  }
});

type PushPayload = { title?: string; body?: string; url?: string };

self.addEventListener('push', (e) => {
  let data: PushPayload = {};
  try {
    data = (e.data?.json() as PushPayload) ?? {};
  } catch {
    /* non-JSON push */
  }
  e.waitUntil(
    self.registration.showNotification(data.title ?? 'Help My Run', {
      body: data.body ?? '',
      icon: '/icons/icon-192.png',
      badge: '/icons/icon-192.png',
      data: { url: data.url ?? '/' },
    }),
  );
});

self.addEventListener('notificationclick', (e) => {
  e.notification.close();
  const url = (e.notification.data as { url?: string } | undefined)?.url ?? '/';
  e.waitUntil(
    self.clients.matchAll({ type: 'window', includeUncontrolled: true }).then((list) => {
      for (const c of list) {
        if ('focus' in c) {
          void c.navigate(url);
          return c.focus();
        }
      }
      return self.clients.openWindow(url);
    }),
  );
});
