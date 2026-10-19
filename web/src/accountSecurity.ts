import { useCallback, useEffect, useState } from 'react'
import { api } from './api'

// Account-security reminder (ADR 088). The server reports which second factors an
// account has; the client only decides whether to show the reminder.

export interface MFAStatus {
  totp_enabled: boolean
  recovery_codes?: boolean
  security_keys: { id: number; name: string; created_at: string; last_used_at?: string }[]
}

// Recovery codes alone are a fallback, not a second factor, so they do not count.
export function hasSecondFactor(status: MFAStatus): boolean {
  return status.totp_enabled || (status.security_keys ?? []).length > 0
}

// The profile page announces MFA changes so the reminder banner can disappear or
// reappear without a reload. A module-level set keeps this free of browser APIs.
const mfaChangeListeners = new Set<() => void>()

export function announceMFAChange() {
  for (const listener of mfaChangeListeners) listener()
}

export function onMFAChange(listener: () => void): () => void {
  mfaChangeListeners.add(listener)
  return () => { mfaChangeListeners.delete(listener) }
}

export type MFAState = 'loading' | 'protected' | 'unprotected' | 'unknown'

// useOwnMFAState loads the signed-in player's MFA status. A failed lookup is
// 'unknown' and shows no reminder: a wrong warning is worse than none.
export function useOwnMFAState(isSignedIn: boolean): MFAState {
  const [state, setState] = useState<MFAState>('loading')
  const load = useCallback(() => {
    if (!isSignedIn) {
      setState('unknown')
      return () => {}
    }
    let isCurrent = true
    api.get<MFAStatus>('/api/v1/auth/mfa')
      .then((status) => { if (isCurrent) setState(hasSecondFactor(status) ? 'protected' : 'unprotected') })
      .catch(() => { if (isCurrent) setState('unknown') })
    return () => { isCurrent = false }
  }, [isSignedIn])
  useEffect(() => {
    let cancelCurrent = load()
    const unsubscribe = onMFAChange(() => {
      cancelCurrent()
      cancelCurrent = load()
    })
    return () => {
      cancelCurrent()
      unsubscribe()
    }
  }, [load])
  return state
}

const dismissKey = 'mfa-reminder-dismissed'

// The reminder can be hidden for the current browser session only; it comes back
// on the next visit until a second factor exists. Storage may be unavailable.
export function isReminderDismissed(): boolean {
  try {
    return window.sessionStorage.getItem(dismissKey) === '1'
  } catch {
    return false
  }
}

export function dismissReminder() {
  try {
    window.sessionStorage.setItem(dismissKey, '1')
  } catch {
    // Without storage the banner is hidden until the next page load only.
  }
}
