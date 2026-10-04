import { useState } from 'react'
import { useSubmission } from './useSubmission'
import { AuthProvider, useAuth } from './auth'
import { Link, useRoute } from './router'
import { atLeast } from './types'
import { LoginPage } from './pages/Login'
import { DashboardPage } from './pages/Dashboard'
import { RaidDetailPage } from './pages/RaidDetail'
import { PlayersPage } from './pages/Players'
import { PlayerDetailPage } from './pages/PlayerDetail'
import { RosterPage } from './pages/Roster'
import { PollsPage } from './pages/Polls'
import { ImportsPage } from './pages/Imports'
import { RaidSeriesPage } from './pages/RaidSeries'

function Routes() {
  const path = useRoute()
  const parts = path.split('/').filter(Boolean)
  if (parts.length === 0) {
    return <DashboardPage />
  }
  if (parts[0] === 'raids' && parts[1]) {
    return <RaidDetailPage raidID={Number(parts[1])} />
  }
  if (parts[0] === 'players' && parts[1]) {
    return <PlayerDetailPage playerID={Number(parts[1])} />
  }
  if (parts[0] === 'players') {
    return <PlayersPage />
  }
  if (parts[0] === 'roster') {
    return <RosterPage />
  }
  if (parts[0] === 'raid-series') return <RaidSeriesPage />
  if (parts[0] === 'polls') {
    return <PollsPage />
  }
  if (parts[0] === 'imports') {
    return <ImportsPage />
  }
  return <p className="muted">Seite nicht gefunden.</p>
}

function Layout() {
  const { player, logout } = useAuth()
  const { pending, submit, isActive } = useSubmission()
  const [error, setError] = useState('')
  const signOut = () => submit(async () => {
    setError('')
    try {
      await logout()
    } catch {
      if (isActive()) setError('Abmelden fehlgeschlagen. Bitte erneut versuchen.')
    }
  })
  if (!player) {
    return null
  }
  return (
    <div className="app">
      <header className="topbar">
        <nav>
          <Link to="/">Raids</Link>
          <Link to="/players">Spieler</Link>
          <Link to="/polls">Umfragen</Link>
          {atLeast(player.role, 'ADMIN') && <Link to="/roster">Roster</Link>}
          {atLeast(player.role, 'ADMIN') && <Link to="/imports">Importe</Link>}
        </nav>
        <div className="user">
          <span>
            {player.name} · <span className="badge">{player.role}</span>
          </span>
          <button disabled={pending} onClick={() => void signOut()}>{pending ? 'Abmelden…' : 'Abmelden'}</button>
        </div>
      </header>
      <main>
        {error && <p className="error" role="alert">{error}</p>}
        <Routes />
      </main>
    </div>
  )
}

function Gate() {
  const { player, loading, error, refresh } = useAuth()
  if (loading) {
    return (
      <div className="login-wrap">
        <p className="muted">Lade…</p>
      </div>
    )
  }
  if (error) {
    return (
      <div className="login-wrap">
        <div className="card login-card">
          <p className="error" role="alert">{error}</p>
          <button onClick={() => void refresh()}>Erneut versuchen</button>
        </div>
      </div>
    )
  }
  if (!player) {
    return <LoginPage />
  }
  return <Layout />
}

export default function App() {
  return (
    <AuthProvider>
      <Gate />
    </AuthProvider>
  )
}
