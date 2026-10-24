// Keinen privaten Inhalt offline speichern: der Service Worker bereitet nur
// die freiwillige PWA-Installation und spätere Opt-in-Benachrichtigungen vor.
self.addEventListener('push', (event) => {
  // Ohne explizite Subscription gibt es keine Push-Ereignisse.
  // Daten aus externen Push-Nachrichten nur als neutrale Benachrichtigung anzeigen.
  event.waitUntil((async () => {
    let kind = ''
    try { kind = event.data?.json()?.type ?? '' } catch { return }
    if (kind !== 'new_survey') return
    await self.registration.showNotification('Neue Umfrage verfügbar', {
      body: 'Mach mit und teile deine Meinung!',
      icon: '/guild-crest.svg',
      data: { url: '/polls' },
    })
  })())
})

self.addEventListener('notificationclick', (event) => {
  event.notification.close()
  const path = event.notification.data?.url === '/polls' ? '/polls' : '/'
  event.waitUntil(clients.openWindow(path))
})
