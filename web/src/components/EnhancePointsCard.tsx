import { Link } from '../router'
import type { PointsResponse } from '../types'

const transactionLabels: Record<string, string> = {
  award_no_mainspec_loot: 'Raidbonus ohne Main-Spec-Loot', correction_no_mainspec_loot: 'Korrektur des Raidbonus',
  legacy_open: 'Historischer Eröffnungssaldo', import_opening: 'Importierter Eröffnungssaldo',
  raid_reward: 'Raidbonus', raid_completion: 'Raidbonus', enhance_roll: 'Historischer Enhance-Einsatz',
  enhanced_loot: 'Enhanced-Loot', enhanced_loot_refund: 'Enhanced-Erstattung',
  adjustment: 'Korrektur', import: 'Import', opening_balance: 'Eröffnungssaldo',
  raid_correction: 'Raidkorrektur', loot_correction: 'Lootkorrektur',
}

export function EnhancePointsCard({ points, showHistory = false }: { points: PointsResponse | null; showHistory?: boolean }) {
  const enhancement = points ? Math.max(0, Math.min(points.balance, 50)) : null
  return <section className="card ep-card">
    <div className="section-heading"><h2>Enhance Points</h2><span className="badge info">EP</span></div>
    <p className="stat">{points?.balance ?? '—'} <span>EP</span></p>
    <div className="ep-preview"><span>Externe Wurfspanne</span><strong>{enhancement === null ? '—' : enhancement > 0 ? `${1 + enhancement}–${100 + enhancement}` : 'Keine Verstärkung'}</strong></div>
    <p className="muted">{enhancement === null ? 'Saldo wird geladen.' : enhancement > 0 ? `Vorschau mit ${enhancement} EP (maximal 50).` : 'Kein positiver EP-Saldo. Kein Enhance-Einsatz möglich; Kosten 0 EP.'} Verlorene Würfe kosten keine EP. Nur der Gewinner zahlt; die Raidleitung erfasst das gewonnene Loot.</p>
    <p className="ep-rule">+10 EP bei der Archivierung ohne Main-Spec-Loot.</p>
    {showHistory && <div className="ledger"><h3>EP-Historie</h3><div className="table-scroll"><table className="table small">
      <thead><tr><th>EP</th><th>Buchung / Begründung</th><th>Raid</th><th>Datum</th></tr></thead>
      <tbody>{points?.transactions.map((t) => <tr key={t.id}>
        <td className={t.amount > 0 ? 'positive' : t.amount < 0 ? 'negative' : 'muted'}>{t.amount > 0 ? `+${t.amount}` : t.amount}</td>
        <td><strong>{transactionLabels[t.type] ?? 'EP-Buchung'}</strong>{t.reason && <div className="muted">{t.reason}</div>}<small className="muted">Akteur #{t.actor_id} · Buchung #{t.id}</small></td>
        <td>{t.raid_id ? <Link to={`/raids/${t.raid_id}`}>Raid #{t.raid_id}</Link> : '—'}</td>
        <td>{new Date(t.created_at).toLocaleString('de-DE')}</td>
      </tr>)}{points && points.transactions.length === 0 && <tr><td colSpan={4} className="muted">Noch keine EP-Buchungen.</td></tr>}</tbody>
    </table></div></div>}
  </section>
}
