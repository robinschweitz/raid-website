import { useEffect, useState, type FormEvent } from 'react'
import { api, ApiError } from '../api'
import { useAuth } from '../auth'
import { atLeast, type Poll, type Player, type Vote } from '../types'

export function PollsPage() {
  const { player: me } = useAuth()
  const [polls, setPolls] = useState<Poll[]>([])
  const [players, setPlayers] = useState<Player[]>([])
  const [votes, setVotes] = useState<Record<number, Vote[]>>({})
  const [error, setError] = useState('')
  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')
  const [choice, setChoice] = useState<Record<number, string>>({})

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
    setError('')
    try {
      await api.post<Poll>('/api/v1/polls', { title, description })
      setTitle('')
      setDescription('')
      load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Anlegen fehlgeschlagen')
    }
  }

  const vote = async (poll: Poll) => {
    const value = choice[poll.id]
    if (!value) return
    setError('')
    try {
      await api.post<Vote>(`/api/v1/polls/${poll.id}/vote`, { choice: value })
      loadVotes(poll.id)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Abstimmen fehlgeschlagen')
    }
  }

  const loadVotes = (pollID: number) => {
    api
      .get<Vote[]>(`/api/v1/polls/${pollID}/votes`)
      .then((vs) => setVotes((prev) => ({ ...prev, [pollID]: vs })))
      .catch(() => undefined)
  }

  const closePoll = async (poll: Poll) => {
    setError('')
    try {
      await api.post<Poll>(`/api/v1/polls/${poll.id}/close`)
      load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Schließen fehlgeschlagen')
    }
  }

  return (
    <div>
      <h1>Umfragen</h1>
      {error && <p className="error">{error}</p>}

      {polls.map((p) => (
        <section className="card" key={p.id}>
          <h2>
            {p.title} <span className="badge">{p.status}</span>
          </h2>
          {p.description && <p className="muted">{p.description}</p>}
          <div className="actions">
            {canVote && p.status === 'OPEN' && (
              <>
                <input
                  placeholder="Antwort"
                  value={choice[p.id] ?? ''}
                  onChange={(e) => setChoice((prev) => ({ ...prev, [p.id]: e.target.value }))}
                />
                <button onClick={() => vote(p)}>Abstimmen</button>
              </>
            )}
            <button onClick={() => loadVotes(p.id)}>Stimmen</button>
            {canManage && p.status === 'OPEN' && <button onClick={() => closePoll(p)}>Schließen</button>}
          </div>
          {votes[p.id] && (
            <ul className="plain-list">
              {votes[p.id].map((v) => (
                <li key={v.id}>
                  {playerName(v.player_id)}: <strong>{v.choice}</strong>
                </li>
              ))}
              {votes[p.id].length === 0 && <li className="muted">Noch keine Stimmen.</li>}
            </ul>
          )}
        </section>
      ))}
      {polls.length === 0 && <p className="muted">Keine Umfragen.</p>}

      {canManage && (
        <form className="card" onSubmit={createPoll}>
          <h2>Neue Umfrage</h2>
          <label>
            Titel
            <input value={title} onChange={(e) => setTitle(e.target.value)} required />
          </label>
          <label>
            Beschreibung
            <input value={description} onChange={(e) => setDescription(e.target.value)} />
          </label>
          <button type="submit">Anlegen</button>
        </form>
      )}
    </div>
  )
}
