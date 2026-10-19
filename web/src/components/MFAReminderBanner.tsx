import { useState } from 'react'
import { dismissReminder, isReminderDismissed, useOwnMFAState } from '../accountSecurity'
import { Link } from '../router'
import { NavIcon } from './NavIcon'

// A thin reminder for signed-in players without a second factor (ADR 088).
export function MFAReminderBanner({ isSignedIn }: { isSignedIn: boolean }) {
  const mfaState = useOwnMFAState(isSignedIn)
  const [isDismissed, setIsDismissed] = useState(isReminderDismissed)
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
