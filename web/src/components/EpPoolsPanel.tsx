import { useState, type FormEvent } from 'react'
import { api, ApiError } from '../api'
import type { EpPool } from '../types'
import { useSubmission } from '../useSubmission'
import { ErrorMessage } from './ErrorMessage'

/** Admin list of EP pools: create and rename, never delete (FR-16.10, ADR 113). */
export function EpPoolsPanel({ pools, onChanged }: { pools: EpPool[]; onChanged: () => void }) {
  const { pending, submit, isActive } = useSubmission()
  const [newName, setNewName] = useState('')
  const [editingID, setEditingID] = useState<number | null>(null)
  const [editName, setEditName] = useState('')
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')

  const save = (event: FormEvent, id: number | null, name: string) => {
    event.preventDefault()
    if (!name.trim()) return
    void submit(async () => {
      setError('')
      setNotice('')
      try {
        if (id === null) await api.post<EpPool>('/api/v1/ep-pools', { name: name.trim() })
        else await api.patch<EpPool>(`/api/v1/ep-pools/${id}`, { name: name.trim() })
        if (!isActive()) return
        setNotice(id === null ? 'EP-Topf angelegt.' : 'EP-Topf umbenannt.')
        setNewName('')
        setEditingID(null)
        onChanged()
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'EP-Topf konnte nicht gespeichert werden.')
      }
    })
  }

  return <section className="card ep-pools">
    <h2>EP-Töpfe</h2>
    <p className="muted">Jeder Topf ist ein eigener EP-Stand. Raidserien wählen ihren Topf; mehrere Serien können sich einen teilen. Töpfe lassen sich nicht löschen.</p>
    {error && <ErrorMessage>{error}</ErrorMessage>}
    {notice && <p className="notice" role="status">{notice}</p>}
    <ul>{pools.map((pool) => <li key={pool.id}>
      {editingID === pool.id
        ? <form className="inline-form" onSubmit={(event) => save(event, pool.id, editName)}>
          <label>Name<input aria-label={`Neuer Name für ${pool.name}`} disabled={pending} required maxLength={60} value={editName} onChange={(e) => setEditName(e.target.value)} /></label>
          <button disabled={pending || !editName.trim()} type="submit">Speichern</button>
          <button disabled={pending} type="button" onClick={() => setEditingID(null)}>Abbrechen</button>
        </form>
        : <>{pool.name} <button disabled={pending} type="button" onClick={() => { setEditingID(pool.id); setEditName(pool.name) }}>Umbenennen</button></>}
    </li>)}</ul>
    <form className="inline-form" onSubmit={(event) => save(event, null, newName)}>
      <label>Neuer EP-Topf<input aria-label="Name des neuen EP-Topfs" disabled={pending} maxLength={60} placeholder="z. B. Ulduar" value={newName} onChange={(e) => setNewName(e.target.value)} /></label>
      <button disabled={pending || !newName.trim()} type="submit">EP-Topf anlegen</button>
    </form>
  </section>
}
