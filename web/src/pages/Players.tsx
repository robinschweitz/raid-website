import { useEffect, useState, type FormEvent } from 'react'
import { api, ApiError } from '../api'
import { useAuth } from '../auth'
import { Link } from '../router'
import { characterClassName } from '../characterTheme'
import { atLeast, type Player, type PlayerAccount, type PlayerSummary, type Role } from '../types'
import { useSubmission } from '../useSubmission'
import { ErrorMessage } from '../components/ErrorMessage'

const ROLES: Role[] = ['GUEST', 'MEMBER', 'RAIDASSISTENT', 'RAIDLEADER', 'ADMIN']

type AccountFilter = 'all' | 'with' | 'without'

const formatDate = (value: string) => new Date(value).toLocaleDateString('de-DE')

// Admins see who can log in and through which registration (ADR 084).
function accountCell(account: PlayerAccount) {
  if (account.origin === 'NONE') return <span className="badge warn">Kein Konto</span>
  return <span className="player-account">
    <strong>@{account.username}</strong>
    <small>{account.origin === 'REGISTERED'
      ? `registriert am ${account.registered_at ? formatDate(account.registered_at) : '—'} über ${account.registered_character}`
      : 'Login vom Admin angelegt'}</small>
  </span>
}

export function PlayersPage() {
  const { pending, submit, isActive } = useSubmission()
  const { player: me } = useAuth()
  const [players, setPlayers] = useState<PlayerSummary[]>([])
  const [error, setError] = useState('')
  const [name, setName] = useState('')
  const [search, setSearch] = useState('')
  const [role, setRole] = useState<Role>('GUEST')
  const [accounts, setAccounts] = useState<PlayerAccount[]>([])
  const [accountFilter, setAccountFilter] = useState<AccountFilter>('all')

  const isAdmin = me ? atLeast(me.role, 'ADMIN') : false

  const load = () => {
    api
      .get<PlayerSummary[]>('/api/v1/players')
      .then(setPlayers)
      .catch((err) => setError(err instanceof ApiError ? err.message : 'Laden fehlgeschlagen'))
    if (isAdmin) {
      api
        .get<PlayerAccount[]>('/api/v1/player-accounts')
        .then(setAccounts)
        .catch((err) => setError(err instanceof ApiError ? err.message : 'Konten konnten nicht geladen werden'))
    }
  }

  useEffect(load, [isAdmin])

  const onCreate = async (event: FormEvent) => {
    event.preventDefault()
    if (!isAdmin) return
    await submit(async () => {
      setError('')
      try {
        await api.post<Player>('/api/v1/players', { name, role })
        if (!isActive()) return
        setName('')
        setRole('GUEST')
        load()
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Anlegen fehlgeschlagen')
      }
    })
  }

  const accountByPlayer = new Map(accounts.map((account) => [account.player_id, account]))
  const query = search.trim().toLowerCase()
  // Admins also find a player by character name or login name.
  const matches = (p: PlayerSummary) => {
    if (p.name.toLowerCase().includes(query)) return true
    const account = accountByPlayer.get(p.id)
    if (!account) return false
    return (account.username ?? '').toLowerCase().includes(query) || account.characters.some((c) => c.name.toLowerCase().includes(query))
  }
  const hasLogin = (p: PlayerSummary) => (accountByPlayer.get(p.id)?.origin ?? 'NONE') !== 'NONE'
  const visiblePlayers = players
    .filter(matches)
    .filter((p) => accountFilter === 'all' || (accountFilter === 'with') === hasLogin(p))
    .sort((a, b) => a.name.localeCompare(b.name, 'de', { numeric: true, sensitivity: 'base' }))
  const showAccounts = isAdmin && accounts.length > 0
  const withLogin = players.filter(hasLogin).length
  const columns = showAccounts ? 5 : 2

  return (
    <div>
      <div className="page-heading"><div><p className="eyebrow">Unsere Gemeinschaft</p><h1>Spieler</h1></div>{isAdmin && <Link to="/roster">Gildenkonfiguration →</Link>}</div>
      {error && <ErrorMessage>{error}</ErrorMessage>}
      {isAdmin && (
        <details className="card editor-disclosure"><summary>Neuer Spieler</summary>
        <form className="card inline-form" onSubmit={onCreate}>
          <label>
            Name
            <input value={name} onChange={(e) => setName(e.target.value)} required />
          </label>
          <label>
            Rolle
            <select value={role} onChange={(e) => setRole(e.target.value as Role)}>
              {ROLES.map((r) => (
                <option key={r} value={r}>
                  {r}
                </option>
              ))}
            </select>
          </label>
          <button type="submit" disabled={pending}>{pending ? 'Anlegen…' : 'Anlegen'}</button>
        </form>
        </details>
      )}
      {showAccounts && <p className="player-account-summary" role="status"><strong>{withLogin} von {players.length}</strong> Spielern haben ein Konto. {players.length - withLogin} haben sich noch nicht registriert.</p>}
      {players.length > 0 && <div className="roster-filters player-search">
        <label className="filter-search">Spieler suchen<input type="search" value={search} placeholder={showAccounts ? 'Name, Charakter oder Login' : 'Name'} onChange={(e) => setSearch(e.target.value)} /></label>
        {showAccounts && <label>Konto
          <select aria-label="Konto" value={accountFilter} onChange={(e) => setAccountFilter(e.target.value as AccountFilter)}>
            <option value="all">Alle Spieler</option>
            <option value="with">Mit Konto</option>
            <option value="without">Ohne Konto</option>
          </select>
        </label>}
        <span className="muted">{visiblePlayers.length} von {players.length} Spielern</span>
      </div>}
      <div className="card table-scroll"><table className="table">
        <thead>
          <tr>
            <th>Name</th>
            {showAccounts && <th>Charaktere</th>}
            {showAccounts && <th>Konto</th>}
            {showAccounts && <th>Rolle</th>}
            <th>Status</th>
          </tr>
        </thead>
        <tbody>
          {visiblePlayers.map((p) => {
            const account = accountByPlayer.get(p.id)
            return (
              <tr key={p.id}>
                <td>
                  <Link to={`/players/${p.id}`}>{p.name}</Link>
                </td>
                {showAccounts && <td className="player-characters">{account && account.characters.length > 0
                  ? account.characters.map((c) => <span key={c.id} className={characterClassName(c.class)}>{c.name}{c.is_main ? ' (Main)' : ''}</span>)
                  : <span className="muted">—</span>}</td>}
                {showAccounts && <td>{account ? accountCell(account) : '—'}</td>}
                {showAccounts && <td>{account ? <code>{account.role}</code> : '—'}</td>}
                <td>{p.active ? 'aktiv' : 'inaktiv'}</td>
              </tr>
            )
          })}
          {players.length === 0 && <tr><td colSpan={columns} className="muted">Keine Spieler geladen.</td></tr>}
          {players.length > 0 && visiblePlayers.length === 0 && <tr><td colSpan={columns} className="muted">Kein Spieler passt zu Suche und Filter.</td></tr>}
        </tbody>
      </table></div>

    </div>
  )
}
