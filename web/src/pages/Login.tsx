import { useState, type FormEvent } from 'react'
import { useAuth } from '../auth'
import { api, ApiError } from '../api'
import { useSubmission } from '../useSubmission'
import { GuildCrest } from '../components/GuildCrest'
import type { ExternalCharacter, Player, RegistrationRequest } from '../types'

export function LoginPage() {
  const { login, refresh } = useAuth()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [code, setCode] = useState('')
  const [regUser, setRegUser] = useState('')
  const [regPass, setRegPass] = useState('')
  const [query, setQuery] = useState('')
  const [results, setResults] = useState<ExternalCharacter[]>([])
  const [selected, setSelected] = useState<ExternalCharacter | null>(null)
  const [mode, setMode] = useState<'login' | 'request' | 'redeem'>('login')
  const [requestStatus, setRequestStatus] = useState('')
  const [error, setError] = useState('')
  const { pending: busy, submit, isActive } = useSubmission()

  const onSubmit = async (event: FormEvent) => {
    event.preventDefault()
    await submit(async () => {
      setError('')
      try {
        await login(username, password)
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Anmelden fehlgeschlagen. Bitte erneut versuchen.')
      }
    })
  }

  const searchCharacters = async () => {
    if (query.trim().length < 3) { setError('Bitte mindestens 3 Zeichen eingeben.'); return }
    await submit(async () => {
      setError('')
      setRequestStatus('')
      setSelected(null)
      try {
        const rows = await api.get<ExternalCharacter[]>(`/api/v1/external/rising-gods/characters?q=${encodeURIComponent(query.trim())}`)
        if (isActive()) setResults(rows)
        if (isActive() && rows.length === 0) setError('Charakter nicht gefunden. Nur aktive Rising-Gods-Charaktere können registriert werden.')
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Charaktersuche fehlgeschlagen.')
      }
    })
  }

  const requestRegistration = async (event: FormEvent) => {
    event.preventDefault()
    if (!selected) return
    await submit(async () => {
      setError('')
      setRequestStatus('')
      try {
        const req = await api.post<RegistrationRequest>('/api/v1/auth/registration-requests', { profile_id: selected.profile_id })
        if (isActive()) setRequestStatus(`Anfrage für ${req.character_name} erstellt. Ein Admin sendet dir den Code ingame an diesen Charakter.`)
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Registrierungsanfrage fehlgeschlagen.')
      }
    })
  }

  const redeem = async (event: FormEvent) => {
    event.preventDefault()
    await submit(async () => {
      setError('')
      setRequestStatus('')
      try {
        await api.post<Player>('/api/v1/auth/register-with-code', { code, username: regUser, password: regPass })
        await refresh()
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Registrierung fehlgeschlagen.')
      }
    })
  }

  return (
    <div className="login-wrap">
      <section className="card login-card login-panel">
        <GuildCrest />
        <p className="eyebrow">Bratwurst des Schicksals</p>
        <h1>Raidverwaltung</h1>
        <div className="login-tabs" role="tablist" aria-label="Anmeldung">
          <button type="button" className={mode === 'login' ? 'active' : ''} onClick={() => setMode('login')}>Anmelden</button>
          <button type="button" className={mode === 'request' ? 'active' : ''} onClick={() => setMode('request')}>Code anfordern</button>
          <button type="button" className={mode === 'redeem' ? 'active' : ''} onClick={() => setMode('redeem')}>Registrieren</button>
        </div>
        {mode === 'login' && <form className="login-section" onSubmit={onSubmit}>
          <label>
            Benutzername
            <input value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="username" autoFocus />
          </label>
          <label>
            Passwort
            <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" />
          </label>
          <button type="submit" disabled={busy}>{busy ? 'Anmelden…' : 'Anmelden'}</button>
        </form>}
        {mode === 'request' && <div className="login-section">
          <p className="muted">Suche deinen Rising-Gods-Charakter. Ein Admin sendet dir danach einen Code ingame an genau diesen Charakter.</p>
          <div className="login-search">
            <label>Charakter suchen<input value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Mindestens 3 Zeichen" /></label>
            <button type="button" disabled={busy || query.trim().length < 3} onClick={() => void searchCharacters()}>Suchen</button>
          </div>
          {results.length > 0 && <div className="login-results">{results.map((row) => <button key={row.profile_id} type="button" className={selected?.profile_id === row.profile_id ? 'active' : ''} onClick={() => setSelected(row)}>
            <strong>{row.name}</strong><span>{row.class} · Stufe {row.level || '—'}</span>
          </button>)}</div>}
          <form className="login-action-row" onSubmit={requestRegistration}>
            <button type="submit" disabled={busy || !selected}>Anfrage erstellen</button>
            {selected && <p className="muted">Ausgewählt: {selected.name} · {selected.class}</p>}
          </form>
        </div>}
        {mode === 'redeem' && <form className="login-section" onSubmit={redeem}>
          <p className="muted">Code erhalten? Lege damit dein Konto an.</p>
          <label>Code<input value={code} onChange={(e) => setCode(e.target.value)} required /></label>
          <label>Benutzername<input value={regUser} onChange={(e) => setRegUser(e.target.value)} autoComplete="username" required /></label>
          <label>Passwort<input type="password" value={regPass} onChange={(e) => setRegPass(e.target.value)} autoComplete="new-password" required /></label>
          <button type="submit" disabled={busy || !code.trim() || !regUser.trim() || !regPass.trim()}>Registrieren</button>
        </form>}
        {error && <p className="error">{error}</p>}
        {requestStatus && <p className="muted" role="status">{requestStatus}</p>}
      </section>
    </div>
  )
}
