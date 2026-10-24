import { useEffect, useState } from 'react'
import { useSubmission } from './useSubmission'
import { AuthProvider, useAuth } from './auth'
import { Link, navigate, useRoute } from './router'
import { atLeast } from './types'
import { LoginPage } from './pages/Login'
import { DashboardPage } from './pages/Dashboard'
import { RaidDetailPage } from './pages/RaidDetail'
import { PlayersPage } from './pages/Players'
import { PlayerDetailPage } from './pages/PlayerDetail'
import { CharacterDetailPage } from './pages/CharacterDetail'
import { RosterPage } from './pages/Roster'
import { PollsPage } from './pages/Polls'
import { RaidSeriesPage } from './pages/RaidSeries'
import { RaidsPage } from './pages/Raids'
import { StatisticsPage } from './pages/Statistics'
import { GuildCrest } from './components/GuildCrest'
import { NavIcon } from './components/NavIcon'
import { LootCatalogPage } from './pages/LootCatalog'
import { RegistrationRequestsPage } from './pages/RegistrationRequests'
import { ChangelogPage } from './pages/Changelog'
import { AuditLogPage } from './pages/AuditLog'
import { WishlistPage } from './pages/Wishlist'
import { GuidePage } from './pages/Guide'
import { ShadowmournePage } from './pages/Shadowmourne'
import { AddonsPage } from './pages/Addons'
import { AccountSecurityPage } from './pages/AccountSecurity'
import { MFAReminderBanner } from './components/MFAReminderBanner'
import { CurrentRaidPage } from './pages/CurrentRaid'
import { appVersion } from './release'
import { ErrorMessage } from './components/ErrorMessage'
import { PWAInstall } from './components/PWAInstall'

// After signing in from the login link the address is still /login.
function RedirectHome() {
  useEffect(() => { navigate('/') }, [])
  return null
}

function Routes() {
  const path = useRoute()
  const parts = path.split('/').filter(Boolean)
  if (parts.length === 0) {
    return <DashboardPage />
  }
  if (parts[0] === 'login') return <RedirectHome />
  if (parts[0] === 'wishlist') return <WishlistPage />
  if (parts[0] === 'raids' && parts[1] === 'current') return <CurrentRaidPage />
  if (parts[0] === 'raids' && parts[1]) {
    return <RaidDetailPage raidID={Number(parts[1])} initialView={parts[2] === 'anmeldungen' ? 'signups' : undefined} />
  }
  if (parts[0] === 'raids') return <RaidsPage />
  if (parts[0] === 'players' && parts[1]) {
    return <PlayerDetailPage playerID={Number(parts[1])} initialView={parts[2] === 'konto' ? 'account' : undefined} />
  }
  if (parts[0] === 'players') {
    return <PlayersPage />
  }
  if (parts[0] === 'characters' && parts[1]) {
    return <CharacterDetailPage characterID={Number(parts[1])} />
  }
  if (parts[0] === 'roster') {
    return <RosterPage />
  }
  if (parts[0] === 'raid-series') return <RaidSeriesPage />
  if (parts[0] === 'statistics') return <StatisticsPage />
  if (parts[0] === 'loot-catalog') return <LootCatalogPage />
  if (parts[0] === 'polls') {
    return <PollsPage />
  }
  if (parts[0] === 'registration-requests') return <RegistrationRequestsPage />
  if (parts[0] === 'audit-log') return <AuditLogPage />
  if (parts[0] === 'changelog') return <ChangelogPage />
  if (parts[0] === 'anleitung') return <GuidePage />
  if (parts[0] === 'schattengram') return <ShadowmournePage />
  if (parts[0] === 'addons') return <AddonsPage addonID={parts[1]} />
  if (parts[0] === 'kontosicherheit') return <AccountSecurityPage />
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
          <Link className={path.startsWith('/raids') && path !== '/raids/current' ? 'nav-link active' : 'nav-link'} to="/raids"><NavIcon name="raids" />Raids</Link>
          <Link className={path === '/raids/current' ? 'nav-link active' : 'nav-link'} to="/raids/current"><NavIcon name="zap" />Aktueller Raid</Link>
          <Link className={path.startsWith('/players') ? 'nav-link active' : 'nav-link'} to="/players"><NavIcon name="players" />Spieler</Link>
          <Link className={path === '/statistics' ? 'nav-link active' : 'nav-link'} to="/statistics"><NavIcon name="chart" />Statistiken</Link>
          <Link className={path === '/polls' ? 'nav-link active' : 'nav-link'} to="/polls"><NavIcon name="polls" />Umfragen</Link>
          <Link className={path === '/loot-catalog' ? 'nav-link active' : 'nav-link'} to="/loot-catalog"><NavIcon name="loot" />Beute</Link>
          <Link className={path === '/schattengram' ? 'nav-link active' : 'nav-link'} to="/schattengram"><NavIcon name="sword" />Schattengram</Link>
          {player && atLeast(player.role, 'MEMBER') && <Link className={path === '/wishlist' ? 'nav-link active' : 'nav-link'} to="/wishlist"><NavIcon name="heart" />Meine Wunschliste</Link>}
          <Link className={path.startsWith('/addons') ? 'nav-link active' : 'nav-link'} to="/addons"><NavIcon name="download" />Addons</Link>
          <Link className={path === '/anleitung' ? 'nav-link active' : 'nav-link'} to="/anleitung"><NavIcon name="info" />Anleitung</Link>
          {player && atLeast(player.role, 'ADMIN') && <>
            <p className="sidebar-caption">Administration</p>
            <Link className={path === '/roster' ? 'nav-link admin-nav active' : 'nav-link admin-nav'} to="/roster"><NavIcon name="settings" />Gildenkonfiguration</Link>
            <Link className={path === '/raid-series' ? 'nav-link admin-nav active' : 'nav-link admin-nav'} to="/raid-series"><NavIcon name="series" />Raidserien</Link>
            <Link className={path === '/registration-requests' ? 'nav-link admin-nav active' : 'nav-link admin-nav'} to="/registration-requests"><NavIcon name="players" />Registrierung</Link>
            <Link className={path === '/audit-log' ? 'nav-link admin-nav active' : 'nav-link admin-nav'} to="/audit-log"><NavIcon name="fileText" />Audit-Protokoll</Link>
          </>}
        </nav>
        <div className="sidebar-footer">
          <Link to="/changelog">Version {appVersion}</Link>
          <span>Änderungen ansehen</span>
        </div>
      </aside>
      <div className="main-shell">
      <header className="topbar">
        <span className="topbar-brand">Bratwurst des Schicksals <span className="muted">/ Raidverwaltung</span></span>
        <div className="user">{player ? <><Link to={`/players/${player.id}`}><NavIcon name="profile" />{player.name}</Link><span className="badge">{player.role}</span>
          <button disabled={pending} onClick={() => void signOut()}>{pending ? 'Abmelden…' : 'Abmelden'}</button></> : <Link to="/login">Anmelden</Link>}</div>
      </header>
      {path !== '/kontosicherheit' && <MFAReminderBanner isSignedIn={player !== null} adminID={player?.role === 'ADMIN' ? player.id : undefined} path={path} />}
      <PWAInstall />
      <main id="main-content" className={path === '/' ? undefined : 'secondary-page'} tabIndex={-1}>
        {error && <ErrorMessage>{error}</ErrorMessage>}
        <Routes />
      </main>
      </div>
      <nav className="mobile-nav" aria-label="Mobile Hauptnavigation">
        <Link className={path === '/' ? 'active' : ''} to="/"><NavIcon name="home" />&Uuml;bersicht</Link>
        <Link className={path.startsWith('/raids') && path !== '/raids/current' ? 'active' : ''} to="/raids"><NavIcon name="raids" />Raids</Link>
        <Link className={path === '/raids/current' ? 'active' : ''} to="/raids/current"><NavIcon name="zap" />Aktuell</Link>
        <Link className={path.startsWith('/players') ? 'active' : ''} to="/players"><NavIcon name="players" />Spieler</Link>
        <Link className={path === '/statistics' ? 'active' : ''} to="/statistics"><NavIcon name="chart" />Statistik</Link>
        <Link className={path === '/polls' ? 'active' : ''} to="/polls"><NavIcon name="polls" />Umfragen</Link>
        <Link className={path === '/loot-catalog' ? 'active' : ''} to="/loot-catalog"><NavIcon name="loot" />Beute</Link>
        <Link className={path === '/schattengram' ? 'active' : ''} to="/schattengram"><NavIcon name="sword" />Schattengram</Link>
        {player && atLeast(player.role, 'MEMBER') && <Link className={path === '/wishlist' ? 'active' : ''} to="/wishlist"><NavIcon name="heart" />Wunschliste</Link>}
        <Link className={path === '/anleitung' ? 'active' : ''} to="/anleitung"><NavIcon name="info" />Hilfe</Link>
      </nav>
    </div>
  )
}

function Gate() {
  const path = useRoute()
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
          <ErrorMessage>{error}</ErrorMessage>
          <button onClick={() => void refresh()}>Erneut versuchen</button>
        </div>
      </div>
    )
  }
  // The guide, the addons pages and the account-security page must be readable before an
  // account exists (ADR 083, ADR 085, ADR 092).
  if (!player && path !== '/' && path !== '/anleitung' && path !== '/kontosicherheit' && path !== '/addons' && !path.startsWith('/addons/')) {
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
