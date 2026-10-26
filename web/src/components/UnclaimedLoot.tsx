import { useEffect, useState, type FormEvent } from 'react'
import { api, ApiError } from '../api'
import { useSubmission } from '../useSubmission'
import type { CatalogBoss } from '../types'
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

interface Props {
  raidID: number
  canWrite: boolean
  needsReason: boolean
  bosses: CatalogBoss[]
  version: number
  mode?: 'all' | 'form' | 'list'
  onChanged?: () => void
}

export function UnclaimedLoot({ raidID, canWrite, needsReason, bosses, version, mode = 'all', onChanged }: Props) {
  const { pending, submit, isActive } = useSubmission()
  const [entries, setEntries] = useState<UnclaimedEntry[]>([])
  const [error, setError] = useState('')
  const [bossID, setBossID] = useState('')
  const [itemName, setItemName] = useState('')
  const [outcome, setOutcome] = useState<Outcome>('NOBODY')
  const [holder, setHolder] = useState('')
  const [notes, setNotes] = useState('')
  const [reason, setReason] = useState('')

  const load = async () => {
    const rows = await api.get<UnclaimedEntry[]>(`/api/v1/raids/${raidID}/unclaimed-loot`)
    if (isActive()) setEntries(rows)
  }

  useEffect(() => {
    void load().catch(() => { if (isActive()) setError('Beute ohne Empfänger konnte nicht geladen werden.') })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [raidID, version])

  const add = (event: FormEvent) => {
    event.preventDefault()
    void submit(async () => {
      setError('')
      try {
        await api.post(`/api/v1/raids/${raidID}/unclaimed-loot`, { catalog_boss_id: bossID, item_name: itemName, outcome, holder_name: holder, notes, reason })
        setItemName('')
        setHolder('')
        setNotes('')
        setReason('')
        await load()
        onChanged?.()
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Konnte nicht gespeichert werden.')
      }
    })
  }

  const remove = (entry: UnclaimedEntry) => submit(async () => {
    setError('')
    try {
      await api.post(`/api/v1/unclaimed-loot/${entry.id}/remove`, { reason })
      setReason('')
      await load()
      onChanged?.()
    } catch (err) {
      if (isActive()) setError(err instanceof ApiError ? err.message : 'Konnte nicht entfernt werden.')
    }
  })

  if (mode === 'form' && !canWrite) return null
  if (mode !== 'form' && !canWrite && entries.length === 0) return null
  return <section className={mode === 'form' ? 'unclaimed-loot' : 'unclaimed-loot loot-unclaimed-list'} aria-label="Beute ohne Empfänger">
    {mode !== 'form' && <h3>Beute ohne Empfänger</h3>}
    {mode !== 'list' && <p className="muted">Entzaubert, für die Gildenbank oder von niemandem gewollt. Zählt für keinen Spieler und kostet keine EP.</p>}
    {error && <ErrorMessage>{error}</ErrorMessage>}
    {canWrite && needsReason && <label className="unclaimed-reason">Grund (Pflicht im archivierten Raid)<input value={reason} onChange={(e) => setReason(e.target.value)} /></label>}
    {mode !== 'form' && (entries.length === 0 ? <p className="muted">Noch nichts eingetragen.</p>
      : <div className="table-scroll"><table className="table small">
        <thead><tr><th>Item</th><th>Boss</th><th>Verbleib</th><th>Bei</th><th>Notizen</th>{canWrite && <th></th>}</tr></thead>
        <tbody>{entries.map((entry) => <tr key={entry.id}>
          <td><strong>{entry.item_name}</strong></td>
          <td>{entry.boss || '—'}</td>
          <td><span className={`badge unclaimed-${entry.outcome.toLowerCase()}`}>{outcomeLabels[entry.outcome]}</span></td>
          <td>{entry.holder_name || '—'}</td>
          <td>{entry.notes}{entry.source === 'addon' && <span className="muted"> (Addon)</span>}</td>
          {canWrite && <td><button type="button" disabled={pending || (needsReason && !reason.trim())} onClick={() => void remove(entry)}>Entfernen</button></td>}
        </tr>)}</tbody>
      </table></div>)}
    {canWrite && mode !== 'list' && <div className={mode === 'form' ? '' : 'editor-disclosure'}>
      {mode !== 'form' && <h4>Beute ohne Empfänger eintragen</h4>}
      <form className="unclaimed-form" onSubmit={add}>
        <label>Boss<select value={bossID} onChange={(e) => setBossID(e.target.value)}>
          <option value="">Ohne Boss</option>
          {bosses.map((boss) => <option key={boss.id} value={boss.id}>{boss.name}</option>)}
        </select></label>
        <label>Item<input value={itemName} required placeholder="Name des Gegenstands" onChange={(e) => setItemName(e.target.value)} /></label>
        <label>Verbleib<select value={outcome} onChange={(e) => setOutcome(e.target.value as Outcome)}>
          {Object.entries(outcomeLabels).map(([value, label]) => <option key={value} value={value}>{label}</option>)}
        </select></label>
        <label>Bei<input value={holder} placeholder="Wer hat es (optional)" onChange={(e) => setHolder(e.target.value)} /></label>
        <label>Notizen<input value={notes} onChange={(e) => setNotes(e.target.value)} /></label>
        <div className="actions"><button type="submit" disabled={pending || !itemName.trim() || (needsReason && !reason.trim())}>Eintragen</button></div>
      </form>
    </div>}
  </section>
}
