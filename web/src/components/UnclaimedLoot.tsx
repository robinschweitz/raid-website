import { useEffect, useState } from 'react'
import { api, ApiError } from '../api'
import { useSubmission } from '../useSubmission'
import { ErrorMessage } from './ErrorMessage'

// Loot that went to nobody (ADR 089): disenchanted, for the guild bank, or
// nobody wanted it. It counts for no player and costs no EP.
type Outcome = 'DISENCHANTED' | 'BANK' | 'NOBODY'

interface UnclaimedEntry {
  id: number
  boss: string
  item_id: string
  item_name: string
  outcome: Outcome
  holder_name: string
  notes: string
  source: 'manual' | 'addon'
}

const outcomeLabels: Record<Outcome, string> = { DISENCHANTED: 'Entzaubert', BANK: 'Gildenbank', NOBODY: 'Wollte niemand' }

interface ListProps {
  raidID: number
  canWrite: boolean
  needsReason: boolean
  version: number
}

// UnclaimedLootList shows the raid's items that went to nobody. Staff can
// remove an entry; the version prop reloads the list after an add or import.
export function UnclaimedLootList({ raidID, canWrite, needsReason, version }: ListProps) {
  const { pending, submit, isActive } = useSubmission()
  const [entries, setEntries] = useState<UnclaimedEntry[]>([])
  const [error, setError] = useState('')
  const [reason, setReason] = useState('')

  const load = async () => {
    const rows = await api.get<UnclaimedEntry[]>(`/api/v1/raids/${raidID}/unclaimed-loot`)
    if (isActive()) setEntries(rows)
  }

  useEffect(() => {
    void load().catch(() => { if (isActive()) setError('Beute ohne Empfänger konnte nicht geladen werden.') })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [raidID, version])

  const remove = (entry: UnclaimedEntry) => submit(async () => {
    setError('')
    try {
      await api.post(`/api/v1/unclaimed-loot/${entry.id}/remove`, { reason })
      setReason('')
      await load()
    } catch (err) {
      if (isActive()) setError(err instanceof ApiError ? err.message : 'Konnte nicht entfernt werden.')
    }
  })

  if (!canWrite && entries.length === 0) return null
  return <section className="unclaimed-loot loot-unclaimed-list" aria-label="Beute ohne Empfänger">
    <h3>Beute ohne Empfänger</h3>
    {error && <ErrorMessage>{error}</ErrorMessage>}
    {entries.length === 0 ? <p className="muted">Noch nichts eingetragen. Neue Einträge über „Loot erfassen“ mit der Vergabemethode Entzaubern, Gildenbank oder Wollte niemand.</p> : <>
      {canWrite && needsReason && <label className="unclaimed-reason">Grund zum Entfernen (Pflicht im archivierten Raid)<input value={reason} onChange={(e) => setReason(e.target.value)} /></label>}
      <div className="table-scroll"><table className="table small">
        <thead><tr><th>Item</th><th>Boss</th><th>Verbleib</th><th>Bei</th><th>Notizen</th>{canWrite && <th></th>}</tr></thead>
        <tbody>{entries.map((entry) => <tr key={entry.id}>
          <td><strong>{entry.item_name}</strong></td>
          <td>{entry.boss || '—'}</td>
          <td><span className={`badge unclaimed-${entry.outcome.toLowerCase()}`}>{outcomeLabels[entry.outcome]}</span></td>
          <td>{entry.holder_name || '—'}</td>
          <td>{entry.notes}{entry.source === 'addon' && <span className="muted"> (Addon)</span>}</td>
          {canWrite && <td><button type="button" disabled={pending || (needsReason && !reason.trim())} onClick={() => void remove(entry)}>Entfernen</button></td>}
        </tr>)}</tbody>
      </table></div>
    </>}
  </section>
}
