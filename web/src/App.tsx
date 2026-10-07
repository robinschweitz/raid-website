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
import { RaidsPage } from './pages/Raids'
import { GuildCrest } from './components/GuildCrest'
import { NavIcon } from './components/NavIcon'
import { LootCatalogPage } from './pages/LootCatalog'

function Routes() {
  const path = useRoute()
  const parts = path.split('/').filter(Boolean)
  if (parts.length === 0) {
    return <DashboardPage />
  }
  if (parts[0] === 'raids' && parts[1]) {
    return <RaidDetailPage raidID={Number(parts[1])} />
  }
  if (parts[0] === 'raids') return <RaidsPage />
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
  if (parts[0] === 'loot-catalog') return <LootCatalogPage />
  if (parts[0] === 'polls') {
    return <PollsPage />
  }
  if (parts[0] === 'imports') {
    return <ImportsPage />
  }
  return <p className="muted">Seite nicht gefunden.</p>
}

function Layout() {
  const path = useRoute()
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
      <a className="skip-link" href="#main-content" onClick={(event) => {
        event.preventDefault()
        document.getElementById('main-content')?.focus()
      }}>Zum Inhalt</a>
      <aside className="sidebar">
        <Link className="guild-brand" to="/"><GuildCrest /><span>Bratwurst<small>des Schicksals</small></span></Link>
        <p className="sidebar-caption">Gildenverwaltung</p>
        <nav aria-label="Hauptnavigation">
          <Link className={path === '/' ? 'nav-link active' : 'nav-link'} to="/"><NavIcon name="home" />&Uuml;bersicht</Link>
          <Link className={path.startsWith('/raids') ? 'nav-link active' : 'nav-link'} to="/raids"><NavIcon name="raids" />Raids</Link>
          <Link className={path.startsWith('/players') ? 'nav-link active' : 'nav-link'} to="/players"><NavIcon name="players" />Spieler</Link>
          <Link className={path === '/polls' ? 'nav-link active' : 'nav-link'} to="/polls"><NavIcon name="polls" />Umfragen</Link>
          <p className="sidebar-caption">Administration</p>
          <Link className={path === '/loot-catalog' ? 'nav-link admin-nav active' : 'nav-link admin-nav'} to="/loot-catalog"><NavIcon name="settings" />Beutekatalog</Link>
          {atLeast(player.role, 'ADMIN') && <>
            <Link className={path === '/roster' ? 'nav-link admin-nav active' : 'nav-link admin-nav'} to="/roster"><NavIcon name="settings" />Gildenkonfiguration</Link>
            <Link className={path === '/raid-series' ? 'nav-link admin-nav active' : 'nav-link admin-nav'} to="/raid-series"><NavIcon name="series" />Raidserien</Link>
            <Link className={path === '/imports' ? 'nav-link admin-nav active' : 'nav-link admin-nav'} to="/imports"><NavIcon name="import" />Importe</Link>
          </>}
        </nav>
      </aside>
      <div className="main-shell">
      <header className="topbar">
        <span className="topbar-brand">Bratwurst des Schicksals <span className="muted">/ Raidverwaltung</span></span>
        <div className="user"><Link to={`/players/${player.id}`}><NavIcon name="profile" />{player.name}</Link><span className="badge">{player.role}</span>
          <button disabled={pending} onClick={() => void signOut()}>{pending ? 'Abmelden…' : 'Abmelden'}</button>
        </div>
      </header>
      <main id="main-content" className={path === '/' ? undefined : 'secondary-page'} tabIndex={-1}>
        {error && <p className="error" role="alert">{error}</p>}
        <Routes />
      </main>
      </div>
      <nav className="mobile-nav" aria-label="Mobile Hauptnavigation">
        <Link className={path === '/' ? 'active' : ''} to="/"><NavIcon name="home" />&Uuml;bersicht</Link>
        <Link className={path.startsWith('/raids') ? 'active' : ''} to="/raids"><NavIcon name="raids" />Raids</Link>
        <Link className={path.startsWith('/players') ? 'active' : ''} to="/players"><NavIcon name="players" />Spieler</Link>
        <Link className={path === '/polls' ? 'active' : ''} to="/polls"><NavIcon name="polls" />Umfragen</Link>
      </nav>
    </div>
  )
}

function Gate() {
  const { player, loading, error, refresh } = useAuth()
  if (loading) {
    return (
      <div className="login-wrap">
        <p className="muted">Lade&hellip;</p>
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
