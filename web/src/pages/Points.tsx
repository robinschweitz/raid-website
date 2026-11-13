import { useEffect, useState } from 'react'
import { api, ApiError } from '../api'
import { useAuth } from '../auth'
import { Link } from '../router'
import { ErrorMessage } from '../components/ErrorMessage'
import { PoolSelect } from '../components/PoolSelect'
import { useEpPools } from '../epPools'

/** The public EP standing of one pool: balances only, never single transactions (FR-16.9, ADR 110, 113). */
interface PublicBalance {
  player_id: number
  player_name: string
  balance: number
}

export function PointsPage() {
  const { player } = useAuth()
  const [rows, setRows] = useState<PublicBalance[] | null>(null)
  const [error, setError] = useState('')
  const pools = useEpPools()
  const [poolID, setPoolID] = useState<number | null>(null)

  useEffect(() => {
    let active = true
    setRows(null)
    setError('')
    api.get<PublicBalance[]>(poolID === null ? '/api/v1/points' : `/api/v1/points?pool_id=${poolID}`)
      .then((data) => { if (active) setRows(data) })
      .catch((err) => { if (active) setError(err instanceof ApiError ? err.message : 'EP-Stand konnte nicht geladen werden.') })
    return () => { active = false }
  }, [poolID])

  return <div className="points-page">
    <div className="page-heading"><div><p className="eyebrow">Enhance Points</p><h1>EP-Stand</h1></div></div>
    <p className="muted">Aktueller EP-Stand aller aktiven Spieler. {player ? 'Die einzelnen Buchungen stehen im Profil des Spielers.' : 'Die einzelnen Buchungen sieht man nach der Anmeldung im Profil des Spielers.'}</p>
    <PoolSelect pools={pools} value={poolID} onChange={setPoolID} />
    {error && <ErrorMessage>{error}</ErrorMessage>}
    {!rows && !error && <p className="muted" role="status">EP-Stand wird geladen…</p>}
    {rows && <section className="card">
      {rows.length === 0 ? <p className="muted">Noch keine aktiven Spieler.</p> : <div className="table-scroll"><table className="table">
        <thead><tr><th>#</th><th>Spieler</th><th>EP</th></tr></thead>
        <tbody>{rows.map((row, index) => <tr key={row.player_id}>
          <td>{index + 1}</td>
          <td>{player ? <Link to={`/players/${row.player_id}`}>{row.player_name}</Link> : row.player_name}</td>
          <td><strong>{row.balance}</strong></td>
        </tr>)}</tbody>
      </table></div>}
    </section>}
  </div>
}
