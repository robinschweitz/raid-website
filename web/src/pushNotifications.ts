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

// Whether this browser gets messages for the signed-in player. The opt-in
// belongs to a player on a device (FR-32.3, ADR 103): the browser checks in with
// its subscription, the server keeps it for the same player and removes it
// when it belongs to someone else. The browser's own subscription is not
// enough; the server decides.
export function pushActiveOnThisDevice(): Promise<boolean> {
  return runExclusive(async () => {
    const subscription = await currentPushSubscription()
    if (!subscription) return false
    const status = await api.post<{ active: boolean }>('/api/v1/push/subscriptions/sync', { endpoint: subscription.endpoint })
    return status.active
  })
}

// How long a sign-in waits for the push check-in before it gives up on it.
const checkInTimeoutMs = 5000

function withTimeout<T>(promise: Promise<T>, ms: number): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error('timeout')), ms)
    promise.then((value) => { clearTimeout(timer); resolve(value) }, (err) => { clearTimeout(timer); reject(err) })
  })
}

export type CheckInOutcome = 'none' | 'active' | 'dropped'

// checkInSubscription keeps this browser's subscription only when the server
// confirms it belongs to the signed-in player. Anything else drops it in the
// browser: a subscription of another player, an unknown one, and also a
// check-in that failed or took too long. It fails closed, so a previous
// player's opt-in can never stay active for the person using the browser now
// (FR-32.3, ADR 103). Signing in itself never depends on it.
export async function checkInSubscription(
  subscription: { endpoint: string; unsubscribe: () => Promise<boolean> } | null,
  ask: (endpoint: string) => Promise<boolean>,
  timeoutMs: number,
): Promise<CheckInOutcome> {
  if (!subscription) return 'none'
  let active = false
  try {
    active = await withTimeout(ask(subscription.endpoint), timeoutMs)
  } catch {
    active = false
  }
  if (active) return 'active'
  await subscription.unsubscribe().catch(() => false)
  return 'dropped'
}

// checkInPush runs before the web client shows a player as signed in, after
// every login, MFA step, security key, and session load. The server already
// removes another player's subscription during the login when it knows the
// device; this step covers a browser that lost its device cookie. It never
// throws.
export async function checkInPush(): Promise<void> {
  if (!browserHasPushAPI()) return
  try {
    await runExclusive(async () => {
      const subscription = await withTimeout(currentPushSubscription(), checkInTimeoutMs).catch(() => null)
      await checkInSubscription(subscription, async (endpoint) => {
        const status = await api.post<{ active: boolean }>('/api/v1/push/subscriptions/sync', { endpoint })
        return status.active
      }, checkInTimeoutMs)
    })
  } catch {
    // Nothing to keep: checkInSubscription already dropped what it could not confirm.
  }
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
