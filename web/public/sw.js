// Service worker for the optional installation (FR-28.5, ADR 101). It has no
// fetch handler and keeps no cache, so no page or API response is stored on the
// device and every request goes to the server as without it. Push messages are
// not handled; that needs its own decision and review first.
self.addEventListener('install', () => {
  self.skipWaiting()
})

self.addEventListener('activate', (event) => {
  event.waitUntil(self.clients.claim())
})
