import { useEffect, useState } from 'react'
import { api, ApiError } from '../api'
import type { Raid } from '../types'

// "Aktueller Raid" (ADR 094): the server picks the running raid, otherwise
// the next upcoming raid; this page only forwards to it. The forward replaces the
// history entry, so the back button does not land here again.
export function CurrentRaidPage() {
  const [message, setMessage] = useState('Aktueller Raid wird gesucht …')
  useEffect(() => {
    let active = true
    api.get<Raid>('/api/v1/raids/current').then((raid) => {
      if (active) window.location.replace(`#/raids/${raid.id}`)
    }).catch((err: unknown) => {
      if (!active) return
      if (err instanceof ApiError && err.code === 'no_current_raid') setMessage('Es gibt noch keinen Raid.')
      else setMessage('Der aktuelle Raid konnte nicht geladen werden. Bitte später erneut versuchen.')
    })
    return () => { active = false }
  }, [])
  return <p className="muted" role="status">{message}</p>
}
