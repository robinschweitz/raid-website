import { useEffect, useState } from 'react'
import { api } from '../api'
import { Link } from '../router'
import type { PointsResponse } from '../types'
import { useEpPools } from '../epPools'
import { PoolSelect } from './PoolSelect'

const transactionLabels: Record<string, string> = {
  award_no_mainspec_loot: 'Raidbonus ohne Main-Spec-Loot', correction_no_mainspec_loot: 'Korrektur des Raidbonus',
  legacy_open: 'Historischer Eröffnungssaldo', import_opening: 'Importierter Eröffnungssaldo',
  raid_reward: 'Raidbonus', raid_completion: 'Raidbonus', enhance_roll: 'Historischer Enhance-Einsatz',
  enhanced_loot: 'Enhanced-Loot', enhanced_loot_refund: 'Enhanced-Erstattung',
  adjustment: 'Korrektur', import: 'Import', opening_balance: 'Eröffnungssaldo',
  raid_correction: 'Raidkorrektur', loot_correction: 'Lootkorrektur',
}

/** Balance, roll range and history of one EP pool; the viewer may switch pools (FR-16.12). */
export function EnhancePointsCard({ points: defaultPoints, showHistory = false }: { points: PointsResponse | null; showHistory?: boolean }) {
  const pools = useEpPools()
  const [poolID, setPoolID] = useState<number | null>(null)
  const [poolPoints, setPoolPoints] = useState<PointsResponse | null>(null)
  const playerID = defaultPoints?.player_id
  useEffect(() => {
    if (poolID === null || playerID === undefined || poolID === defaultPoints?.pool_id) { setPoolPoints(null); return }
    let active = true
    setPoolPoints(null)
    api.get<PointsResponse>(`/api/v1/players/${playerID}/points?pool_id=${poolID}`)
      .then((data) => { if (active) setPoolPoints(data) })
      .catch(() => { if (active) setPoolPoints(null) })
    return () => { active = false }
  }, [poolID, playerID, defaultPoints?.pool_id])
  const points = poolID === null || poolID === defaultPoints?.pool_id ? defaultPoints : poolPoints
  const enhancement = points ? Math.max(0, Math.min(points.balance, 50)) : null
  return <section className="card ep-card">
    <div className="section-heading"><h2>Enhance Points</h2><span className="badge info">EP</span></div>
    <PoolSelect pools={pools} value={poolID ?? defaultPoints?.pool_id ?? null} onChange={setPoolID} />
    <p className="stat">{points?.balance ?? '—'} <span>EP</span></p>
    <div className="ep-preview"><span>Enhanced-Wurf</span><strong>{enhancement === null ? '—' : enhancement > 0 ? `${1 + enhancement}–${100 + enhancement}` : 'Nicht möglich'}</strong></div>
    <p className="muted">{enhancement === null ? 'EP-Stand wird geladen.' : enhancement > 0 ? `Es zählen ${enhancement} EP (höchstens 50).` : 'Ohne EP ist kein Enhanced-Wurf möglich.'} Verlorene Würfe kosten keine EP. Nur der Gewinner zahlt; die Raidleitung erfasst das gewonnene Loot.</p>
    <p className="ep-rule">EP gibt es bei der Archivierung ohne Main-Spec-Loot; wie viele, legt die Raidserie fest (meist +10).</p>
    {showHistory && <div className="ledger"><h3>EP-Historie</h3><div className="table-scroll"><table className="table small">
      <thead><tr><th>EP</th><th>Buchung / Begründung</th><th>Raid</th><th>Datum</th></tr></thead>
      <tbody>{points?.transactions.map((t) => <tr key={t.id}>
        <td className={t.amount > 0 ? 'positive' : t.amount < 0 ? 'negative' : 'muted'}>{t.amount > 0 ? `+${t.amount}` : t.amount}</td>
        <td><strong>{transactionLabels[t.type] ?? 'EP-Buchung'}</strong>{t.reason && <div className="muted">{t.reason}</div>}</td>
        <td>{t.raid_id ? <Link to={`/raids/${t.raid_id}`}>Raid #{t.raid_id}</Link> : '—'}</td>
        <td>{new Date(t.created_at).toLocaleString('de-DE')}</td>
      </tr>)}{points && points.transactions.length === 0 && <tr><td colSpan={4} className="muted">Noch keine EP-Buchungen.</td></tr>}</tbody>
    </table></div></div>}
  </section>
}
