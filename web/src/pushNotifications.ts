import { api } from './api'

// Opt-in push notifications (FR-32, ADR 102). The browser asks for permission
// only after the player clicks the button, never on page load.

export type PushConfig = { enabled: boolean; public_key: string }

export type PushAvailability = 'unsupported' | 'ios-install' | 'denied' | 'server-off' | 'available'

// Which state the opt-in shows. iPhones and iPads deliver push only to the
// installed web app (iOS 16.4 and newer), so a Safari tab gets the install hint.
export function pushAvailability(state: {
  hasPushAPI: boolean
  isIOS: boolean
  isStandalone: boolean
  permission: NotificationPermission | 'unsupported'
  serverEnabled: boolean
}): PushAvailability {
  if (!state.serverEnabled) return 'server-off'
  if (state.isIOS && !state.isStandalone) return 'ios-install'
  if (!state.hasPushAPI || state.permission === 'unsupported') return 'unsupported'
  if (state.permission === 'denied') return 'denied'
  return 'available'
}

export function browserHasPushAPI(): boolean {
  return typeof window !== 'undefined' && window.isSecureContext && 'serviceWorker' in navigator && 'PushManager' in window && 'Notification' in window
}

// applicationServerKey wants raw bytes, the server sends base64url.
export function base64URLToBytes(value: string): Uint8Array {
  const base64 = (value + '='.repeat((4 - (value.length % 4)) % 4)).replace(/-/g, '+').replace(/_/g, '/')
  const raw = atob(base64)
  const bytes = new Uint8Array(new ArrayBuffer(raw.length))
  for (let i = 0; i < raw.length; i++) bytes[i] = raw.charCodeAt(i)
  return bytes
}

// The fields the server stores; the rest of PushSubscription.toJSON() stays here.
export function subscriptionBody(subscription: PushSubscriptionJSON): { endpoint: string; p256dh: string; auth: string } {
  return { endpoint: subscription.endpoint ?? '', p256dh: subscription.keys?.p256dh ?? '', auth: subscription.keys?.auth ?? '' }
}

export async function currentPushSubscription(): Promise<PushSubscription | null> {
  if (!browserHasPushAPI()) return null
  const registration = await navigator.serviceWorker.getRegistration()
  return registration ? registration.pushManager.getSubscription() : null
}

// Push changes run one after another, so a status read, an enable, a disable
// and the logout clean-up can never interleave and act on a stale browser
// subscription (ADR 103). A failed step does not block the next one.
let pushQueue: Promise<unknown> = Promise.resolve()

export function runExclusive<T>(step: () => Promise<T>): Promise<T> {
  const result = pushQueue.then(step, step)
  pushQueue = result.catch(() => undefined)
  return result
}

// Whether this browser gets messages for the current session. A subscription
// ends on the server with the session that made it and a new session never
// inherits it (FR-32.3, ADR 103), so the browser's own subscription is not
// enough: the server decides.
export function pushActiveOnThisDevice(): Promise<boolean> {
  return runExclusive(async () => {
    const subscription = await currentPushSubscription()
    if (!subscription) return false
    const status = await api.post<{ active: boolean }>('/api/v1/push/subscriptions/status', { endpoint: subscription.endpoint })
    return status.active
  })
}

export async function enablePush(config: PushConfig): Promise<void> {
  // Asked first and outside the queue, so the prompt still belongs to the click.
  const permission = await Notification.requestPermission()
  if (permission !== 'granted') throw new Error(permission === 'denied' ? 'denied' : 'dismissed')
  await runExclusive(async () => {
    const registration = await navigator.serviceWorker.ready
    let subscription = await registration.pushManager.getSubscription()
    if (!subscription) {
      subscription = await registration.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: base64URLToBytes(config.public_key) })
    }
    try {
      await api.post('/api/v1/push/subscriptions', subscriptionBody(subscription.toJSON()))
    } catch (err) {
      // A browser subscription the server does not know would never get a message.
      await subscription.unsubscribe().catch(() => false)
      throw err
    }
  })
}

export function disablePush(): Promise<void> {
  return runExclusive(async () => {
    const subscription = await currentPushSubscription()
    if (!subscription) return
    try {
      await api.del('/api/v1/push/subscriptions', { endpoint: subscription.endpoint })
    } finally {
      await subscription.unsubscribe().catch(() => false)
    }
  })
}

// On logout the device stops getting messages for this account (FR-32.3).
// Failures are ignored so that logging out always works; the server also
// removes the subscription with the session.
export async function disablePushOnLogout(): Promise<void> {
  try {
    await disablePush()
  } catch {
    // The subscription ends with the session on the server anyway.
  }
}
