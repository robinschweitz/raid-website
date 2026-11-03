import { useState } from 'react'
import { api, ApiError } from '../api'
import { useSubmission } from '../useSubmission'
import type { Player } from '../types'
import { ErrorMessage } from './ErrorMessage'

interface Props {
  raidID: number
  leaderIDs: number[]
  players: Player[]
  /** Archived raids need a reason for every change (FR-26.5). */
  requiresReason: boolean
  onSaved: (leaderIDs: number[]) => void
}

/** Staff set the raid leaders of a planned or open raid; they are shown only (FR-10.17). */
export function RaidLeadersEditor({ raidID, leaderIDs, players, requiresReason, onSaved }: Props) {
  const { pending, submit, isActive } = useSubmission()
  const [draft, setDraft] = useState<number[]>(leaderIDs)
  const [candidate, setCandidate] = useState('')
  const [error, setError] = useState('')
  const [reason, setReason] = useState('')
  const name = (id: number) => players.find((p) => p.id === id)?.name ?? `Spieler #${id}`
  const changed = draft.join(',') !== leaderIDs.join(',')

  function save() {
    void submit(async () => {
      setError('')
      try {
        const saved = await api.put<number[]>(`/api/v1/raids/${raidID}/leaders`, { player_ids: draft, reason })
        if (isActive()) onSaved(saved)
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Raidleitung konnte nicht gespeichert werden.')
      }
    })
  }

  return <section className="subcard raid-leaders-editor" aria-label="Raidleitung">
    <h3>Raidleitung</h3>
    <p className="muted">Mehrere Spieler können den Raid leiten. Die Angabe ist nur zur Anzeige; Rechte hängen an der Rolle.</p>
    {error && <ErrorMessage>{error}</ErrorMessage>}
    <ol className="plain-list">
      {draft.map((id) => <li key={id}>
        {name(id)}{' '}
        <button type="button" disabled={pending || draft.length <= 1} onClick={() => setDraft(draft.filter((row) => row !== id))}>Entfernen</button>
      </li>)}
    </ol>
    <div className="actions">
      <label>Raidleitung hinzufügen
        <select value={candidate} disabled={pending} onChange={(event) => setCandidate(event.target.value)}>
          <option value="">Spieler wählen</option>
          {players.filter((p) => p.active && !draft.includes(p.id)).map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
        </select>
      </label>
      <button type="button" disabled={pending || !candidate} onClick={() => { setDraft([...draft, Number(candidate)]); setCandidate('') }}>Hinzufügen</button>
    </div>
    {requiresReason && <label>Begründung (archivierter Raid)
      <input value={reason} maxLength={500} disabled={pending} onChange={(event) => setReason(event.target.value)} placeholder="z. B. Co-Leitung nachgetragen" />
    </label>}
    <div className="actions">
      <button type="button" className="primary-action" disabled={pending || !changed || !draft.length || (requiresReason && !reason.trim())} onClick={save}>{pending ? 'Speichert…' : 'Raidleitung speichern'}</button>
      {changed && <button type="button" disabled={pending} onClick={() => setDraft(leaderIDs)}>Verwerfen</button>}
    </div>
  </section>
}
