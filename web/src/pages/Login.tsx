import { useState, type FormEvent } from 'react'
import { useAuth } from '../auth'
import { ApiError } from '../api'
import { useSubmission } from '../useSubmission'
import { GuildCrest } from '../components/GuildCrest'

export function LoginPage() {
  const { login } = useAuth()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
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

  return (
    <div className="login-wrap">
      <form className="card login-card" onSubmit={onSubmit}>
        <GuildCrest />
        <p className="eyebrow">Bratwurst des Schicksals</p>
        <label>
          Benutzername
          <input value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="username" autoFocus />
        </label>
        <label>
          Passwort
          <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" />
        </label>
        {error && <p className="error">{error}</p>}
        <button type="submit" disabled={busy}>
          {busy ? 'Anmelden&hellip;' : 'Anmelden'}
        </button>
      </form>
    </div>
  )
}
