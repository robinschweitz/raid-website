import { useState, type FormEvent } from 'react'
import { api, ApiError } from '../api'
import { Link } from '../router'
import { ErrorMessage } from '../components/ErrorMessage'
import { useSubmission } from '../useSubmission'

export function ResetPasswordPage() {
  const [code, setCode] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [done, setDone] = useState(false)
  const { pending, submit, isActive } = useSubmission()
  const redeem = async (event: FormEvent) => {
    event.preventDefault()
    if (!code.trim() || !password) return
    await submit(async () => {
      setError('')
      try {
        await api.post('/api/v1/auth/reset-password', { code: code.trim(), password })
        if (!isActive()) return
        setCode('')
        setPassword('')
        setDone(true)
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Das Passwort konnte nicht zurückgesetzt werden.')
      }
    })
  }
  return <section className="card">
    <h1>Passwort zurücksetzen</h1>
    <p>Bitte einen Admin ingame um einen Reset-Code für deinen Main-Charakter. Er gilt 72 Stunden und kann nur einmal eingelöst werden.</p>
    <p>Beim Einlösen werden alle bisherigen Sitzungen beendet und die bisherigen zweiten Faktoren entfernt. Richte danach Authenticator-App oder Sicherheitsschlüssel erneut ein; als Admin ist das erforderlich.</p>
    {error && <ErrorMessage>{error}</ErrorMessage>}
    {done ? <p role="status">Dein Passwort wurde geändert. <a href="/">Jetzt neu anmelden</a>.</p> : <form onSubmit={redeem}>
      <label>Reset-Code<input aria-label="Reset-Code" autoComplete="off" value={code} onChange={(e) => setCode(e.target.value)} required /></label>
      <label>Neues Passwort<input type="password" aria-label="Neues Passwort" autoComplete="new-password" value={password} onChange={(e) => setPassword(e.target.value)} required /></label>
      <p className="field-hint">Mindestens 12 Zeichen, höchstens 1024 Bytes.</p>
      <button type="submit" disabled={pending || !code.trim() || !password}>{pending ? 'Wird zurückgesetzt…' : 'Passwort zurücksetzen'}</button>
    </form>}
    <p><Link to="/">Zur Anmeldung</Link></p>
  </section>
}
