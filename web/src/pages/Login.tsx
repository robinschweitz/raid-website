import { useState, type FormEvent } from 'react'
import { useAuth, type MFAFactors } from '../auth'
import { api, ApiError } from '../api'
import { useSubmission } from '../useSubmission'
import { GuildCrest } from '../components/GuildCrest'
import { Link } from '../router'
import type { ExternalCharacter, Player, RegistrationRequest } from '../types'
import { browserSupportsWebAuthn } from '../webauthn'

export function LoginPage() {
  const { login, verifyMFA, verifySecurityKey, refresh } = useAuth()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [mfaCode, setMfaCode] = useState('')
  const [mfaChallenge, setMfaChallenge] = useState('')
  const [mfaFactors, setMfaFactors] = useState<MFAFactors>({ totp_enabled: false, recovery_codes: false, webauthn: false })
  const [mfaMethod, setMfaMethod] = useState<'webauthn' | 'code'>('code')
  const [code, setCode] = useState('')
  const [regUser, setRegUser] = useState('')
  const [regPass, setRegPass] = useState('')
  const [query, setQuery] = useState('')
  const [results, setResults] = useState<ExternalCharacter[]>([])
  const [selected, setSelected] = useState<ExternalCharacter | null>(null)
  const [mode, setMode] = useState<'login' | 'request' | 'redeem'>('login')
  const [requestStatus, setRequestStatus] = useState('')
  const [error, setError] = useState('')
  const [searching, setSearching] = useState(false)
  const { pending: busy, submit, isActive } = useSubmission()

  const onSubmit = async (event: FormEvent) => {
    event.preventDefault()
    await submit(async () => {
      setError('')
      try {
        const result = await login(username, password)
        if (result.mfaRequired && result.challenge) {
          const factors = result.factors ?? { totp_enabled: false, recovery_codes: false, webauthn: false }
          setMfaChallenge(result.challenge)
          setMfaFactors(factors)
          setMfaMethod(factors.webauthn ? 'webauthn' : 'code')
          setPassword('')
          return
        }
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Anmelden fehlgeschlagen. Bitte erneut versuchen.')
      }
    })
  }

  const onMFASubmit = async (event: FormEvent) => {
    event.preventDefault()
    await submit(async () => {
      setError('')
      try {
        await verifyMFA(mfaChallenge, mfaCode)
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'MFA-Code konnte nicht geprüft werden.')
      }
    })
  }

  const onSecurityKeySubmit = async () => {
    await submit(async () => {
      setError('')
      try {
        await verifySecurityKey(mfaChallenge)
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Sicherheitsschlüssel konnte nicht geprüft werden.')
      }
    })
  }

  const hasCodeMFA = mfaFactors.totp_enabled || mfaFactors.recovery_codes
  const webAuthnSupported = browserSupportsWebAuthn()
  const hasUsableWebAuthn = mfaFactors.webauthn && webAuthnSupported
  const resetMFA = () => {
    setMfaChallenge('')
    setMfaCode('')
    setMfaFactors({ totp_enabled: false, recovery_codes: false, webauthn: false })
    setMfaMethod('code')
  }

  const codePrompt = mfaFactors.totp_enabled && mfaFactors.recovery_codes
    ? 'Bitte gib den sechsstelligen Code aus deiner Authenticator-App oder einen Wiederherstellungscode ein.'
    : mfaFactors.totp_enabled
      ? 'Bitte gib den sechsstelligen Code aus deiner Authenticator-App ein.'
      : 'Bitte gib einen Wiederherstellungscode ein.'

  const searchCharacters = async () => {
    if (query.trim().length < 3) { setError('Bitte mindestens 3 Zeichen eingeben.'); return }
    await submit(async () => {
      setError('')
      setRequestStatus('')
      setSelected(null)
      setSearching(true)
      try {
        const rows = await api.get<ExternalCharacter[]>(`/api/v1/external/rising-gods/characters?q=${encodeURIComponent(query.trim())}`)
        if (isActive()) setResults(rows)
        if (isActive() && rows.length === 0) setError('Charakter nicht gefunden. Nur aktive Rising-Gods-Charaktere können registriert werden.')
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Charaktersuche fehlgeschlagen.')
      } finally {
        if (isActive()) setSearching(false)
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
        {mode === 'login' && !mfaChallenge && <form className="login-section" onSubmit={onSubmit}>
          <label>
            Benutzername
            <input value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="username" autoFocus />
          </label>
          <label>
            Passwort
            <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" />
          </label>
          <button className="primary-action" type="submit" disabled={busy}>{busy ? 'Anmelden…' : 'Anmelden'}</button>
        </form>}
        {mode === 'login' && mfaChallenge && <div className="login-section">
          {mfaFactors.webauthn && mfaMethod === 'webauthn' && <div className="login-section">
            <p className="muted">Melde dich mit deinem Sicherheitsschlüssel an.</p>
            {!webAuthnSupported && <p className="error">Dieser Browser unterstützt WebAuthn nicht. Verwende einen kompatiblen Browser oder ein Gerät mit Sicherheitsschlüssel-Unterstützung.</p>}
            <button type="button" disabled={busy || !hasUsableWebAuthn} onClick={() => void onSecurityKeySubmit()}>{busy ? 'Prüfe…' : 'Mit YubiKey anmelden'}</button>
            {hasCodeMFA && <button type="button" className="ghost" disabled={busy} onClick={() => setMfaMethod('code')}>Stattdessen Code verwenden</button>}
          </div>}
          {hasCodeMFA && mfaMethod === 'code' && <form className="login-section" onSubmit={onMFASubmit}>
            <p className="muted">{codePrompt}</p>
            <label>
              MFA-Code
              <input value={mfaCode} onChange={(e) => setMfaCode(e.target.value)} autoComplete="one-time-code" autoFocus />
            </label>
            <button className="primary-action" type="submit" disabled={busy || !mfaCode.trim()}>{busy ? 'Prüfe…' : 'Anmelden'}</button>
            {hasUsableWebAuthn && <button type="button" className="ghost" disabled={busy} onClick={() => setMfaMethod('webauthn')}>Mit YubiKey anmelden</button>}
          </form>}
          {!mfaFactors.webauthn && !hasCodeMFA && <p className="error">Für dieses Konto ist MFA aktiv, aber es ist kein verwendbarer Faktor verfügbar.</p>}
          <button type="button" className="ghost" disabled={busy} onClick={resetMFA}>Zurück</button>
        </div>}
        {mode === 'request' && <div className="login-section">
          <p className="muted">Suche deinen Rising-Gods-Charakter. Ein Admin sendet dir danach einen Code ingame an genau diesen Charakter.</p>
          <div className="login-search">
            <label>Charakter suchen<input value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Mindestens 3 Zeichen" /></label>
            <button type="button" disabled={busy || query.trim().length < 3} onClick={() => void searchCharacters()}>{searching ? 'Suche…' : 'Suchen'}</button>
          </div>
          {searching && <p className="search-status" role="status"><span className="search-spinner" aria-hidden="true" />Rising-Gods-Suche läuft… Das kann einige Sekunden dauern. Schneller geht es mit einem Rising-Gods-Profil-Link.</p>}
          {results.length > 0 && <div className="login-results">{results.map((row) => <button key={row.profile_id} type="button" className={selected?.profile_id === row.profile_id ? 'active' : ''} onClick={() => { setSelected(row); setRegUser(row.name) }}>
            <strong>{row.name}</strong><span>{row.class} · Stufe {row.level || '—'}</span>
          </button>)}</div>}
          <form className="login-action-row" onSubmit={requestRegistration}>
            <button className="primary-action" type="submit" disabled={busy || !selected}>Anfrage erstellen</button>
            {selected && <p className="muted">Ausgewählt: {selected.name} · {selected.class}</p>}
          </form>
        </div>}
        {mode === 'redeem' && <form className="login-section" onSubmit={redeem}>
          <p className="muted">Code erhalten? Lege damit dein Konto an.</p>
          <label>Code<input value={code} onChange={(e) => setCode(e.target.value)} required /></label>
          <label>Benutzername<input value={regUser} onChange={(e) => setRegUser(e.target.value)} autoComplete="username" required /></label>
          <label>Passwort<input type="password" value={regPass} onChange={(e) => setRegPass(e.target.value)} autoComplete="new-password" required /></label>
          <button className="primary-action" type="submit" disabled={busy || !code.trim() || !regUser.trim() || !regPass.trim()}>Registrieren</button>
        </form>}
        {error && <p className="error">{error}</p>}
        {requestStatus && <p className="muted" role="status">{requestStatus}</p>}
        <p className="login-help">Neu hier? <Link to="/anleitung">Anleitung mit Bildern lesen</Link></p>
      </section>
    </div>
  )
}
