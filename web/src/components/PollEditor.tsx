import { useState, type FormEvent } from 'react'
import type { Poll, PollStatus } from '../types'

export interface PollEditBody {
  title: string
  description: string
  closes_at: string
  status: PollStatus
  options: { id: number; label: string }[]
}

interface Props {
  poll: Poll
  pending: boolean
  onSave: (body: PollEditBody) => void
  onCancel: () => void
}

function localInput(timestamp: string | null): string {
  if (!timestamp) return ''
  const date = new Date(timestamp)
  return new Date(date.getTime() - date.getTimezoneOffset() * 60000).toISOString().slice(0, 16)
}

// PollEditor edits a poll in place (FR-24.7). Options keep their id, so a
// renamed option keeps its votes; the server refuses to drop voted options.
export function PollEditor({ poll, pending, onSave, onCancel }: Props) {
  const [title, setTitle] = useState(poll.title)
  const [description, setDescription] = useState(poll.description)
  const [closesAt, setClosesAt] = useState(localInput(poll.closes_at))
  const [status, setStatus] = useState<PollStatus>(poll.status === 'CLOSED' ? 'CLOSED' : 'OPEN')
  const [options, setOptions] = useState(poll.options.map((option) => ({ id: option.id, label: option.label })))
  const isChoice = poll.poll_type === 'CHOICE'

  const move = (index: number, step: number) => setOptions((rows) => {
    const next = [...rows]
    const [row] = next.splice(index, 1)
    next.splice(index + step, 0, row)
    return next
  })

  function save(event: FormEvent) {
    event.preventDefault()
    onSave({
      title, description, status,
      closes_at: closesAt ? new Date(closesAt).toISOString() : '',
      options: isChoice ? options.map((option) => ({ id: option.id, label: option.label.trim() })).filter((option) => option.id !== 0 || option.label) : [],
    })
  }

  return <form className="poll-editor" onSubmit={save} aria-label={`Umfrage ${poll.title} bearbeiten`}>
    <label>Titel<input value={title} disabled={pending} onChange={(event) => setTitle(event.target.value)} required /></label>
    <label>Beschreibung<input value={description} disabled={pending} onChange={(event) => setDescription(event.target.value)} /></label>
    <label>Ende (optional)<input type="datetime-local" value={closesAt} disabled={pending} onChange={(event) => setClosesAt(event.target.value)} /></label>
    <label>Status
      <select value={status} disabled={pending} onChange={(event) => setStatus(event.target.value as PollStatus)}>
        <option value="OPEN">Offen</option>
        <option value="CLOSED">Geschlossen</option>
      </select>
    </label>
    {isChoice && <fieldset className="poll-editor-options">
      <legend>Antworten</legend>
      <p className="field-hint">Umbenannte Antworten behalten ihre Stimmen. Antworten mit Stimmen lassen sich nicht entfernen.</p>
      {options.map((option, index) => <div className="poll-editor-option" key={option.id || `new-${index}`}>
        <input aria-label={`Antwort ${index + 1}`} value={option.label} disabled={pending} onChange={(event) => {
          const label = event.target.value
          setOptions((rows) => rows.map((row, rowIndex) => rowIndex === index ? { ...row, label } : row))
        }} />
        <button type="button" aria-label={`Antwort ${index + 1} nach oben`} disabled={pending || index === 0} onClick={() => move(index, -1)}>↑</button>
        <button type="button" aria-label={`Antwort ${index + 1} nach unten`} disabled={pending || index === options.length - 1} onClick={() => move(index, 1)}>↓</button>
        <button type="button" disabled={pending || options.length <= 2} onClick={() => setOptions((rows) => rows.filter((_, rowIndex) => rowIndex !== index))}>Entfernen</button>
      </div>)}
      <button type="button" disabled={pending} onClick={() => setOptions((rows) => [...rows, { id: 0, label: '' }])}>Antwort hinzufügen</button>
    </fieldset>}
    <div className="actions">
      <button type="submit" className="primary-action" disabled={pending}>Speichern</button>
      <button type="button" disabled={pending} onClick={onCancel}>Abbrechen</button>
    </div>
  </form>
}
