import { useEffect, useState, type FormEvent } from 'react'
import { api, ApiError } from '../api'
import { useAuth } from '../auth'
import { Link } from '../router'
import { atLeast, type Character, type ShadowmourneQueueEntry } from '../types'
import { useSubmission } from '../useSubmission'

function errorMessage(err: unknown): string {
  return err instanceof ApiError ? err.message : 'Die Schattengram-Liste konnte nicht geladen werden.'
}

function characterLabel(character: Character): string {
  return `${character.name} (${character.class})`
}

export function ShadowmournePage() {
  const { player: me } = useAuth()
  const { pending, submit, isActive } = useSubmission()
  const [entries, setEntries] = useState<ShadowmourneQueueEntry[]>([])
  const [characters, setCharacters] = useState<Character[]>([])
  const [error, setError] = useState('')
  const [characterID, setCharacterID] = useState('')
  const [shardsCollected, setShardsCollected] = useState('0')
  const [shardTarget, setShardTarget] = useState('')
  const [currentShards, setCurrentShards] = useState('0')
  const [currentTarget, setCurrentTarget] = useState('')

  const canManage = me ? atLeast(me.role, 'RAIDASSISTENT') : false
  const active = entries.find((entry) => entry.status === 'COLLECTING')
  const queued = entries.filter((entry) => entry.status === 'QUEUED')
  const completed = entries.filter((entry) => entry.status === 'COMPLETED')

  const load = () => {
    setError('')
    const requests: [Promise<ShadowmourneQueueEntry[]>, Promise<Character[]>] = [
      api.get<ShadowmourneQueueEntry[]>('/api/v1/shadowmourne-queue'),
      canManage ? api.get<Character[]>('/api/v1/characters') : Promise.resolve([]),
    ]
    Promise.all(requests)
      .then(([queue, characterRows]) => {
        if (!isActive()) return
        setEntries(queue)
        setCharacters(characterRows.filter((character) => character.active))
        const collector = queue.find((entry) => entry.status === 'COLLECTING')
        setCurrentShards(collector ? String(collector.shards_collected) : '0')
        setCurrentTarget(collector ? String(collector.shard_target) : '')
      })
      .catch((err) => { if (isActive()) setError(errorMessage(err)) })
  }

  useEffect(load, [canManage, isActive])

  const create = async (event: FormEvent) => {
    event.preventDefault()
    if (!canManage) return
    await submit(async () => {
      setError('')
      try {
        await api.post<ShadowmourneQueueEntry>('/api/v1/shadowmourne-queue', {
          character_id: Number(characterID),
          shards_collected: Number(shardsCollected),
          shard_target: Number(shardTarget),
        })
        if (!isActive()) return
        setCharacterID('')
        setShardsCollected('0')
        setShardTarget('')
        load()
      } catch (err) {
        if (isActive()) setError(errorMessage(err))
      }
    })
  }

  const saveProgress = async (event: FormEvent) => {
    event.preventDefault()
    if (!active || !canManage) return
    await submit(async () => {
      setError('')
      try {
        await api.patch<ShadowmourneQueueEntry>(`/api/v1/shadowmourne-queue/${active.id}`, {
          shards_collected: Number(currentShards),
          shard_target: Number(currentTarget),
        })
        if (isActive()) load()
      } catch (err) {
        if (isActive()) setError(errorMessage(err))
      }
    })
  }

  const complete = async () => {
    if (!active || !canManage || !window.confirm(`${active.character_name} als vollständig abgeschlossen markieren?`)) return
    await submit(async () => {
      setError('')
      try {
        await api.post<ShadowmourneQueueEntry>(`/api/v1/shadowmourne-queue/${active.id}/complete`, {})
        if (isActive()) load()
      } catch (err) {
        if (isActive()) setError(errorMessage(err))
      }
    })
  }

  const move = async (entry: ShadowmourneQueueEntry, direction: 'UP' | 'DOWN') => {
    if (!canManage) return
    await submit(async () => {
      setError('')
      try {
        await api.post<ShadowmourneQueueEntry[]>(`/api/v1/shadowmourne-queue/${entry.id}/move`, { direction })
        if (isActive()) load()
      } catch (err) {
        if (isActive()) setError(errorMessage(err))
      }
    })
  }

  return <div>
    <div className="page-heading"><div><p className="eyebrow">Splitter-Priorität</p><h1>Schattengram</h1></div></div>
    <p className="muted">Ein Charakter sammelt Schattengram vollständig, danach ist der nächste Listenplatz an der Reihe.</p>
    {error && <p className="error" role="alert">{error}</p>}
    {pending && <p className="muted" role="status">Wird gespeichert&hellip;</p>}

    <section className="card">
      <div className="section-heading"><h2>Aktuell gesammelt</h2><span className="badge ok">{active ? 'Sammelt' : 'Niemand'}</span></div>
      {active ? <>
        <p><Link to={`/characters/${active.character_id}`}><strong>{active.character_name}</strong></Link> <span className="muted">von {active.player_name}</span></p>
        <progress value={active.shards_collected} max={active.shard_target}>{active.shards_collected} von {active.shard_target}</progress>
        <p><strong>{active.shards_collected} von {active.shard_target} Splittern</strong></p>
        {canManage && <form className="actions" onSubmit={saveProgress}>
          <label>Gesammelt<input aria-label="Gesammelte Splitter" type="number" min="0" value={currentShards} onChange={(event) => setCurrentShards(event.target.value)} required /></label>
          <label>Ziel<input aria-label="Splitterziel" type="number" min="1" value={currentTarget} onChange={(event) => setCurrentTarget(event.target.value)} required /></label>
          <button type="submit" disabled={pending}>Stand speichern</button>
          <button type="button" disabled={pending || active.shards_collected !== active.shard_target} onClick={() => void complete()}>Abgeschlossen</button>
        </form>}
      </> : <p className="muted">Noch kein Charakter sammelt Schattengram.</p>}
    </section>

    {canManage && <details className="card editor-disclosure"><summary>Charakter einreihen</summary>
      <form className="card" onSubmit={create}>
        <label>Charakter<select aria-label="Schattengram-Charakter" value={characterID} onChange={(event) => setCharacterID(event.target.value)} required><option value="">Bitte wählen</option>{characters.map((character) => <option key={character.id} value={character.id}>{characterLabel(character)}</option>)}</select></label>
        <label>Bereits gesammelt<input aria-label="Neue gesammelte Splitter" type="number" min="0" value={shardsCollected} onChange={(event) => setShardsCollected(event.target.value)} required /></label>
        <label>Splitterziel<input aria-label="Neues Splitterziel" type="number" min="1" value={shardTarget} onChange={(event) => setShardTarget(event.target.value)} required /></label>
        <button type="submit" disabled={pending}>Einreihen</button>
      </form>
    </details>}

    <section className="card">
      <div className="section-heading"><h2>Reihenfolge</h2><span className="eyebrow">Nach Schattengram</span></div>
      <div className="table-scroll"><table className="table"><thead><tr><th>Platz</th><th>Charakter</th><th>Besitzer</th>{canManage && <th>Reihenfolge</th>}</tr></thead><tbody>
        {queued.map((entry, index) => <tr key={entry.id}><td>{index + 1}</td><td><Link to={`/characters/${entry.character_id}`}>{entry.character_name}</Link></td><td>{entry.player_name}</td>{canManage && <td className="actions"><button type="button" disabled={pending || index === 0} onClick={() => void move(entry, 'UP')}>Früher</button><button type="button" disabled={pending || index === queued.length - 1} onClick={() => void move(entry, 'DOWN')}>Später</button></td>}</tr>)}
        {queued.length === 0 && <tr><td colSpan={canManage ? 4 : 3} className="muted">Noch niemand wartet auf einen Platz.</td></tr>}
      </tbody></table></div>
    </section>

    <section className="card">
      <div className="section-heading"><h2>Abgeschlossen</h2><span className="eyebrow">Historie</span></div>
      <div className="table-scroll"><table className="table"><thead><tr><th>Charakter</th><th>Besitzer</th><th>Splitter</th><th>Abgeschlossen</th></tr></thead><tbody>
        {completed.map((entry) => <tr key={entry.id}><td><Link to={`/characters/${entry.character_id}`}>{entry.character_name}</Link></td><td>{entry.player_name}</td><td>{entry.shards_collected} / {entry.shard_target}</td><td>{entry.completed_at ? new Date(entry.completed_at).toLocaleDateString('de-DE') : '-'}</td></tr>)}
        {completed.length === 0 && <tr><td colSpan={4} className="muted">Noch kein Charakter hat Schattengram abgeschlossen.</td></tr>}
      </tbody></table></div>
    </section>
  </div>
}
