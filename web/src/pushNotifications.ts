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
// subscription (ADR 103). A failed step does not block the next one, and no
// step waits longer than waitMs for the one before it, so one stalled browser
// call cannot hold every later push change.
const queueWaitMs = 10000
let pushQueue: Promise<unknown> = Promise.resolve()

function settledOrAfter(previous: Promise<unknown>, ms: number): Promise<void> {
  return new Promise((resolve) => {
    const timer = setTimeout(resolve, ms)
    const done = () => { clearTimeout(timer); resolve() }
    previous.then(done, done)
  })
}

export function runExclusive<T>(step: () => Promise<T>, waitMs = queueWaitMs): Promise<T> {
  const result = settledOrAfter(pushQueue, waitMs).then(step)
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

// The whole push check-in of a sign-in, queue wait included, ends after this.
const checkInDeadlineMs = 5000

type BrowserSubscription = { endpoint: string; unsubscribe: () => Promise<boolean> }

export type CheckInOutcome = 'none' | 'active' | 'dropped' | 'timeout'

export type CheckInDeps = {
  queue: <T>(step: () => Promise<T>) => Promise<T>
  getSubscription: () => Promise<BrowserSubscription | null>
  ask: (endpoint: string, signal: AbortSignal) => Promise<boolean>
  deadlineMs: number
}

// dropInBackground ends a browser subscription without waiting for it: a
// stalled browser call must neither hold the sign-in nor the push queue. The
// push service answers 410 for it later and the server deletes the row.
function dropInBackground(subscription: BrowserSubscription): void {
  void subscription.unsubscribe().catch(() => false)
}

// checkInWithDeadline keeps this browser's subscription only when the server
// confirms it belongs to the signed-in player, within one deadline that covers
// the queue wait, the server request, and the decision. Anything else drops
// it: another player's or an unknown subscription, a failed check-in, and a
// check-in that missed the deadline. At the deadline the request is aborted
// and a late answer changes nothing, so the outcome stays "dropped" (fail
// closed, FR-32.3, ADR 103). Signing in never depends on it.
export async function checkInWithDeadline(deps: CheckInDeps): Promise<CheckInOutcome> {
  const controller = new AbortController()
  let timer: ReturnType<typeof setTimeout> | undefined
  const deadline = new Promise<CheckInOutcome>((resolve) => { timer = setTimeout(() => resolve('timeout'), deps.deadlineMs) })
  const work = deps.queue(async (): Promise<CheckInOutcome> => {
    if (controller.signal.aborted) return 'timeout'
    const subscription = await deps.getSubscription()
    if (!subscription) return 'none'
    let active = false
    try {
      active = await deps.ask(subscription.endpoint, controller.signal)
    } catch {
      active = false
    }
    // Past the deadline the decision is already made; a late answer changes nothing.
    if (controller.signal.aborted) return 'timeout'
    if (active) return 'active'
    dropInBackground(subscription)
    return 'dropped'
  }).catch((): CheckInOutcome => 'dropped')
  const outcome = await Promise.race([work, deadline])
  clearTimeout(timer)
  if (outcome === 'timeout') {
    controller.abort()
    void deps.getSubscription().then((subscription) => { if (subscription) dropInBackground(subscription) }).catch(() => undefined)
  }
  return outcome
}

// checkInPush runs before the web client shows a player as signed in, after
// every login, MFA step, security key, and session load. The server already
// removes another player's subscription during the login when it knows the
// device; this step covers a browser that lost its device cookie. It never
// throws and never takes longer than checkInDeadlineMs.
export async function checkInPush(): Promise<void> {
  if (!browserHasPushAPI()) return
  await checkInWithDeadline({
    queue: (step) => runExclusive(step),
    getSubscription: currentPushSubscription,
    ask: async (endpoint, signal) => {
      const status = await api.post<{ active: boolean }>('/api/v1/push/subscriptions/sync', { endpoint }, { signal })
      return status.active
    },
    deadlineMs: checkInDeadlineMs,
  }).catch(() => undefined)
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
