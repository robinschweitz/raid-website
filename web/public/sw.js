// Service worker for the optional installation (FR-28.5, ADR 101) and the
// opt-in push notifications (FR-32, ADR 102). It has no fetch handler and keeps
// no cache, so no page or API response is stored on the device and every
// request goes to the server as without it (SC-052). A push message carries
// only its kind; the texts below are fixed and name no poll, raid or player.
const messages = {
  poll: { body: 'Es gibt eine neue Umfrage.', url: '/#/polls' },
  raid: { body: 'Es wurde ein neuer Raid geplant.', url: '/#/raids' },
  roster: { body: 'Du stehst in einer Raid-Aufstellung.', url: '/#/' },
}
const fallbackMessage = { body: 'Es gibt Neuigkeiten in RAID.', url: '/#/' }

self.addEventListener('install', () => {
  self.skipWaiting()
})

self.addEventListener('activate', (event) => {
  event.waitUntil(self.clients.claim())
})

function messageFor(data) {
  try {
    const kind = data ? data.json().kind : ''
    return Object.prototype.hasOwnProperty.call(messages, kind) ? { kind, ...messages[kind] } : { kind: 'raid-info', ...fallbackMessage }
  } catch {
    return { kind: 'raid-info', ...fallbackMessage }
  }
}

self.addEventListener('push', (event) => {
  const message = messageFor(event.data)
  event.waitUntil(self.registration.showNotification('RAID', {
    body: message.body,
    icon: '/app-icon-192.png',
    lang: 'de',
    tag: message.kind,
    data: { url: message.url },
  }))
})

self.addEventListener('notificationclick', (event) => {
  event.notification.close()
  const target = new URL((event.notification.data && event.notification.data.url) || '/', self.location.origin).href
  event.waitUntil(self.clients.matchAll({ type: 'window', includeUncontrolled: true }).then((windows) => {
    const open = windows.find((client) => client.url.startsWith(self.location.origin))
    if (!open) return self.clients.openWindow(target)
    return open.focus().then((client) => (client && 'navigate' in client ? client.navigate(target) : client)).catch(() => self.clients.openWindow(target))
  }))
})
