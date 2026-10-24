import { useEffect, useState, type FormEvent } from 'react'
import { api, ApiError } from '../api'
import { useAuth } from '../auth'
import { atLeast, type Poll, type Player, type PollType, type Vote } from '../types'
import { useSubmission } from '../useSubmission'
import { ErrorMessage } from '../components/ErrorMessage'
import { PollEditor, type PollEditBody } from '../components/PollEditor'

function pollEditError(err: unknown): string {
  if (!(err instanceof ApiError)) return 'Speichern fehlgeschlagen'
  if (err.code === 'poll_option_has_votes') return 'Eine Antwort, für die schon abgestimmt wurde, kann nicht entfernt werden. Benenne sie stattdessen um.'
  if (err.message.includes('closes_at')) return 'Eine offene Umfrage braucht ein Ende in der Zukunft oder gar keins.'
  if (err.message.includes('two options')) return 'Eine Auswahl-Umfrage braucht mindestens zwei Antworten.'
  if (err.message.includes('duplicate option')) return 'Jede Antwort darf nur einmal vorkommen.'
  return err.message
}

export function PollsPage() {
  const { pending, submit, isActive } = useSubmission()
  const { player: me } = useAuth()
  const [polls, setPolls] = useState<Poll[]>([])
  const [players, setPlayers] = useState<Player[]>([])
  const [votes, setVotes] = useState<Record<number, Vote[]>>({})
  const [error, setError] = useState('')
  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')
  const [pollType, setPollType] = useState<PollType>('TEXT')
  const [allowMultiple, setAllowMultiple] = useState(false)
  const [optionsText, setOptionsText] = useState('')
  const [choice, setChoice] = useState<Record<number, string>>({})
  const [selectedChoices, setSelectedChoices] = useState<Record<number, string[]>>({})
  const [editingID, setEditingID] = useState<number | null>(null)

  const canManage = me ? atLeast(me.role, 'RAIDASSISTENT') : false
  const canVote = me ? atLeast(me.role, 'MEMBER') : false

  const load = () => {
    setError('')
    Promise.all([api.get<Poll[]>('/api/v1/polls'), api.get<Player[]>('/api/v1/players')])
      .then(([ps, pl]) => {
        setPolls(ps)
        setPlayers(pl)
      })
      .catch((err) => setError(err instanceof ApiError ? err.message : 'Laden fehlgeschlagen'))
  }

  useEffect(load, [])

  const playerName = (id: number) => players.find((p) => p.id === id)?.name ?? `#${id}`

  const createPoll = async (event: FormEvent) => {
    event.preventDefault()
    if (!canManage) return
    await submit(async () => {
      setError('')
      try {
        const options = optionsText.split('\n').map((option) => option.trim()).filter(Boolean)
        await api.post<Poll>('/api/v1/polls', { title, description, poll_type: pollType, allow_multiple: allowMultiple, options })
        if (!isActive()) return
        setTitle('')
        setDescription('')
        setPollType('TEXT')
        setAllowMultiple(false)
        setOptionsText('')
        load()
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Anlegen fehlgeschlagen')
      }
    })
  }

  const vote = async (poll: Poll) => {
    const values = poll.poll_type === 'CHOICE' ? (selectedChoices[poll.id] ?? []) : [choice[poll.id] ?? '']
    if (values.length === 0 || values.every((value) => !value.trim()) || !canVote) return
    await submit(async () => {
      setError('')
      try {
        await api.post<Vote>(`/api/v1/polls/${poll.id}/vote`, poll.poll_type === 'CHOICE' ? { choices: values } : { choice: values[0] })
        if (!isActive()) return
        loadVotes(poll.id)
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Abstimmen fehlgeschlagen')
      }
    })
  }

  const toggleChoice = (pollID: number, label: string, multiple: boolean) => {
    setSelectedChoices((prev) => {
      const current = prev[pollID] ?? []
      if (!multiple) return { ...prev, [pollID]: [label] }
      if (current.includes(label)) return { ...prev, [pollID]: current.filter((choiceLabel) => choiceLabel !== label) }
      return { ...prev, [pollID]: [...current, label] }
    })
  }

  const loadVotes = (pollID: number) => {
    api
      .get<Vote[]>(`/api/v1/polls/${pollID}/votes`)
      .then((vs) => setVotes((prev) => ({ ...prev, [pollID]: vs })))
      .catch((err) => { if (isActive()) setError(err instanceof ApiError ? err.message : 'Stimmen laden fehlgeschlagen') })
  }

  const closePoll = async (poll: Poll) => {
    if (!canManage) return
    await submit(async () => {
      setError('')
      try {
        await api.post<Poll>(`/api/v1/polls/${poll.id}/close`)
        if (!isActive()) return
        load()
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Schließen fehlgeschlagen')
      }
    })
  }

  const editPoll = async (poll: Poll, body: PollEditBody) => {
    if (!canManage) return
    await submit(async () => {
      setError('')
      try {
        await api.put<Poll>(`/api/v1/polls/${poll.id}`, body)
        if (!isActive()) return
        setEditingID(null)
        setVotes((prev) => { const next = { ...prev }; delete next[poll.id]; return next })
        load()
      } catch (err) {
        if (isActive()) setError(pollEditError(err))
      }
    })
  }

  return (
    <div>
      <div className="page-heading"><div><p className="eyebrow">Gemeinsam entscheiden</p><h1>Umfragen</h1></div></div>
      {error && <ErrorMessage>{error}</ErrorMessage>}
      {pending && <p className="muted" role="status">Wird gespeichert…</p>}

      {canManage && (
        <details className="card editor-disclosure"><summary>Neue Umfrage</summary>
        <form className="card" onSubmit={createPoll}>
          <label>
            Titel
            <input value={title} onChange={(e) => setTitle(e.target.value)} required />
          </label>
          <label>
            Beschreibung
            <input value={description} onChange={(e) => setDescription(e.target.value)} />
          </label>
          <label>
            Art
            <select value={pollType} onChange={(e) => { setPollType(e.target.value as PollType); if (e.target.value === 'TEXT') setAllowMultiple(false) }}>
              <option value="TEXT">Freitext</option>
              <option value="CHOICE">Vordefinierte Antworten</option>
            </select>
          </label>
          {pollType === 'CHOICE' && (
            <>
              <label>
                Antworten, eine pro Zeile
                <textarea value={optionsText} onChange={(e) => setOptionsText(e.target.value)} required />
              </label>
              <label className="inline-choice">
                <input type="checkbox" checked={allowMultiple} onChange={(e) => setAllowMultiple(e.target.checked)} />
                Mehrere Antworten erlauben
              </label>
            </>
          )}
          <button type="submit" disabled={pending}>Anlegen</button>
        </form>
        </details>
      )}

      <div className="poll-directory">{polls.map((p) => (
        <section className="card" key={p.id}>
          <h2>
            {p.title} <span className={p.status === 'OPEN' ? 'badge ok' : 'badge'}>{p.status === 'OPEN' ? 'Offen' : 'Geschlossen'}</span>
          </h2>
          {editingID === p.id ? <PollEditor poll={p} pending={pending} onSave={(body) => { void editPoll(p, body) }} onCancel={() => setEditingID(null)} /> : <>
          {p.description && <p className="muted">{p.description}</p>}
          {p.poll_type === 'CHOICE' && <p className="muted">{p.allow_multiple ? 'Mehrfachauswahl möglich' : 'Eine Auswahl möglich'}</p>}
          <div className="actions">
            {canVote && p.status === 'OPEN' && (
              <>
                {p.poll_type === 'CHOICE' ? (
                  <div className="option-stack" role="group" aria-label={`Antwort auf ${p.title}`}>
                    {p.options.map((option) => {
                      const selected = (selectedChoices[p.id] ?? []).includes(option.label)
                      return <label className="inline-choice" key={option.id}>
                        <input
                          type={p.allow_multiple ? 'checkbox' : 'radio'}
                          name={`poll-${p.id}`}
                          checked={selected}
                          onChange={() => toggleChoice(p.id, option.label, p.allow_multiple)}
                        />
                        {option.label}
                      </label>
                    })}
                  </div>
                ) : (
                  <input
                    placeholder="Antwort"
                    aria-label={`Antwort auf ${p.title}`}
                    value={choice[p.id] ?? ''}
                    onChange={(e) => setChoice((prev) => ({ ...prev, [p.id]: e.target.value }))}
                  />
                )}
                <button disabled={pending} onClick={() => vote(p)}>Abstimmen</button>
              </>
            )}
            <button onClick={() => loadVotes(p.id)}>Stimmen</button>
            {canManage && p.status === 'OPEN' && <button disabled={pending} onClick={() => closePoll(p)}>Schließen</button>}
            {canManage && <button disabled={pending} onClick={() => setEditingID(p.id)}>Bearbeiten</button>}
          </div>
          {p.closes_at && <p className="muted">Ende: {new Date(p.closes_at).toLocaleString('de-DE')}</p>}
          </>}
          {votes[p.id] && (
            <ul className="plain-list">
              {votes[p.id].map((v) => (
                <li key={v.id}>
                  {playerName(v.player_id)}: <strong>{v.choices?.length ? v.choices.join(', ') : v.choice}</strong>
                </li>
              ))}
              {votes[p.id].length === 0 && <li className="muted">Noch keine Stimmen.</li>}
            </ul>
          )}
        </section>
      ))}</div>
      {polls.length === 0 && <p className="muted">{canManage ? 'Noch keine Umfragen. Mit „Neue Umfrage“ die erste starten.' : 'Keine Umfragen.'}</p>}

    </div>
  )
}
