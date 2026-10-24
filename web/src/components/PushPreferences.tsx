import { useState } from 'react'
import { api } from '../api'

export function PushPreferences() {
  const [message, setMessage] = useState('')
  const [enabled, setEnabled] = useState(false)

  // No permission request occurs without a direct user click.
  const enable = async () => {
    setMessage('')
    if (!('serviceWorker' in navigator) || !('PushManager' in window)) {
      setMessage('Dieser Browser unterstützt keine Push-Benachrichtigungen.')
      return
    }
    // Delivery is not enabled yet: no misleading browser permission prompt.
    setMessage('Benachrichtigungen werden vorbereitet und sind noch nicht verfügbar.')
  }

  return <section className="card" aria-label="Benachrichtigungen">
    <h2>Benachrichtigungen</h2>
    <p>Neue Umfragen – freiwillig, ohne Tracking und ohne persönliche Inhalte in der Nachricht.</p>
    <button type="button" disabled={enabled} onClick={() => void enable()}>
      Benachrichtigungen einrichten
    </button>
    {message && <p role="status">{message}</p>}
  </section>
}
