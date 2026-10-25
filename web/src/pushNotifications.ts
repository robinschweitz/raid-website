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

// The browser remembers which player turned push on, so that another player
// who signs in on the same browser does not inherit the subscription (FR-32.3).
const pushOwnerKey = 'raid-push-player'

function readPushOwner(): string | null {
  try {
    return window.localStorage.getItem(pushOwnerKey)
  } catch {
    return null
  }
}

function writePushOwner(playerID: number | null): void {
  try {
    if (playerID === null) window.localStorage.removeItem(pushOwnerKey)
    else window.localStorage.setItem(pushOwnerKey, String(playerID))
  } catch {
    // Without storage the subscription is only kept for the player who made it.
  }
}

// What syncPushForPlayer does with this browser's subscription for a signed-in
// player: keep it bound to the current session, drop it because another player
// turned it on, or nothing without a subscription or permission.
export function pushSyncAction(state: { hasSubscription: boolean; owner: string | null; playerID: number; permission: NotificationPermission }): 'none' | 'drop' | 'resubscribe' {
  if (!state.hasSubscription) return 'none'
  if (state.owner !== null && state.owner !== String(state.playerID)) return 'drop'
  if (state.permission !== 'granted') return 'none'
  return 'resubscribe'
}

// A subscription ends on the server with the session that made it (ADR 103).
// On every visit the browser's subscription is therefore sent again, so it is
// bound to the current session of the same player; a subscription another
// player turned on is dropped instead. Failures leave the account tab to show
// the state; they never block the page.
export async function syncPushForPlayer(playerID: number): Promise<void> {
  try {
    const subscription = await currentPushSubscription()
    const action = pushSyncAction({ hasSubscription: subscription !== null, owner: readPushOwner(), playerID, permission: Notification.permission })
    if (!subscription || action === 'none') return
    if (action === 'drop') {
      await subscription.unsubscribe().catch(() => false)
      writePushOwner(null)
      return
    }
    await api.post('/api/v1/push/subscriptions', subscriptionBody(subscription.toJSON()))
    writePushOwner(playerID)
  } catch {
    // The account tab shows whether this device is subscribed.
  }
}

export async function enablePush(config: PushConfig, playerID: number): Promise<void> {
  const permission = await Notification.requestPermission()
  if (permission !== 'granted') throw new Error(permission === 'denied' ? 'denied' : 'dismissed')
  const registration = await navigator.serviceWorker.ready
  let subscription = await registration.pushManager.getSubscription()
  if (!subscription) {
    subscription = await registration.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: base64URLToBytes(config.public_key) })
  }
  try {
    await api.post('/api/v1/push/subscriptions', subscriptionBody(subscription.toJSON()))
    writePushOwner(playerID)
  } catch (err) {
    // A browser subscription the server does not know would never get a message.
    await subscription.unsubscribe().catch(() => false)
    throw err
  }
}

export async function disablePush(): Promise<void> {
  const subscription = await currentPushSubscription()
  if (!subscription) return
  try {
    await api.del('/api/v1/push/subscriptions', { endpoint: subscription.endpoint })
  } finally {
    await subscription.unsubscribe().catch(() => false)
    writePushOwner(null)
  }
}

// On logout the device stops getting messages for this account (FR-32.3).
// Failures are ignored so that logging out always works.
export async function disablePushOnLogout(): Promise<void> {
  try {
    await disablePush()
  } catch {
    // The server drops the subscription later when the push service answers 410.
  }
}
