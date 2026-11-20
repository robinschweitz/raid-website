import { useState } from 'react'
import { api, ApiError } from '../api'
import { storeSupport, supportSession, type SupportSession } from '../supportSession'
import { useSubmission } from '../useSubmission'
import { ErrorMessage } from './ErrorMessage'

export function StartSupport({ id, name }: { id: number; name: string }) {
  const { pending, submit } = useSubmission()
  const [error, setError] = useState('')
  if (supportSession()) return null
  return <><button disabled={pending} onClick={() => void submit(async () => {
    try {
      const v = await api.post<Omit<SupportSession, 'name'>>('/api/v1/support/start', { player_id: id })
      storeSupport({ ...v, name }); window.location.hash = '/'; window.location.reload()
    } catch (err) { setError(err instanceof Error ? err.message : 'Spieleransicht konnte nicht gestartet werden.') }
  })}>Als Spieler ansehen</button>{error && <ErrorMessage>{error}</ErrorMessage>}</>
}

export function SupportBanner() {
  const [view, setView] = useState(supportSession)
  const [error, setError] = useState('')
  const { pending, submit } = useSubmission()
  if (!view) return null
  function exit(target: number) { storeSupport(null); window.location.hash = `/players/${target}`; window.location.reload() }
  return <aside className="support-banner" aria-label="Admin-Stellvertretung"><strong>{view.writable ? 'Du handelst für' : 'Nur ansehen:'} {view.name}</strong>
    {!view.writable && <button disabled={pending} onClick={() => void submit(async () => {
      try { await api.post('/api/v1/support/write'); const next = { ...view, writable: true }; storeSupport(next); setView(next); setError('') }
      catch (err) { setError(err instanceof Error ? err.message : 'Schreibmodus nicht verfügbar.') }
    })}>Als Spieler handeln</button>}
    <button disabled={pending} onClick={() => void submit(async () => {
      try { await api.post('/api/v1/support/end'); exit(view.target) }
      catch (err) { if (err instanceof ApiError && (err.code === 'support_expired' || err.status === 401 || err.status === 403)) exit(view.target); else setError('Rückkehr fehlgeschlagen. Bitte erneut versuchen.') }
    })}>Zurück zum Admin</button>{error && <ErrorMessage>{error}</ErrorMessage>}</aside>
}
