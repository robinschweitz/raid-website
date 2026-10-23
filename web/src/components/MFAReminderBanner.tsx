import { useEffect, useRef, useState } from 'react'
import { dismissReminder, isReminderDismissed, useOwnMFAState } from '../accountSecurity'
import { Link, navigate } from '../router'
import { NavIcon } from './NavIcon'

interface MFAReminderBannerProps {
  isSignedIn: boolean
  // Set for an ADMIN: Admin functions need a second factor (FR-3.10, SC-040).
  adminID?: number
  path?: string
}

// A thin reminder for signed-in players without a second factor (ADR 092). For an
// Admin it is required: it cannot be hidden, links to the account tab, and the first
// page after login is that tab (FR-3.10).
export function MFAReminderBanner({ isSignedIn, adminID, path }: MFAReminderBannerProps) {
  const mfaState = useOwnMFAState(isSignedIn)
  const [isDismissed, setIsDismissed] = useState(isReminderDismissed)
  const hasRedirected = useRef(false)
  const accountPath = adminID ? `/players/${adminID}/konto` : ''
  const isRequired = adminID !== undefined && mfaState === 'unprotected'

  useEffect(() => {
    if (!isRequired || hasRedirected.current) return
    hasRedirected.current = true
    if (path !== accountPath) navigate(accountPath)
  }, [isRequired, path, accountPath])

  if (isRequired) {
    return <div className="mfa-reminder mfa-reminder-required" role="alert">
      <NavIcon name="shield" />
      <Link className="mfa-reminder-link" to={accountPath}>
        <strong>Als Admin brauchst du einen zweiten Faktor.</strong>
        <span> Richte eine Authenticator-App oder einen Sicherheitsschlüssel ein; bis dahin sind die Admin-Funktionen gesperrt. Jetzt einrichten →</span>
      </Link>
    </div>
  }
  if (mfaState !== 'unprotected' || isDismissed) return null
  return <div className="mfa-reminder" role="note" aria-label="Hinweis zur Kontosicherheit">
    <NavIcon name="shield" />
    <Link className="mfa-reminder-link" to="/kontosicherheit">
      <strong>Dein Konto ist nur mit einem Passwort geschützt.</strong>
      <span> Ein zweiter Faktor stoppt laut Microsoft über 99,9 % der Kontoübernahmen. Warum das wichtig ist →</span>
    </Link>
    <button type="button" className="mfa-reminder-close" aria-label="Hinweis für diese Sitzung ausblenden" title="Für diese Sitzung ausblenden" onClick={() => {
      dismissReminder()
      setIsDismissed(true)
    }}><NavIcon name="x" /></button>
  </div>
}
