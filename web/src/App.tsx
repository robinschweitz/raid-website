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
          <button onClick={() => void logout()}>Abmelden</button>
        </div>
      </header>
      <main>
        <Routes />
      </main>
    </div>
  )
}

function Gate() {
  const { player, loading } = useAuth()
  if (loading) {
    return (
      <div className="login-wrap">
        <p className="muted">Lade…</p>
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
