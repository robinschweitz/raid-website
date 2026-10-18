import { useEffect, useState, type FormEvent } from 'react'
import { api, ApiError } from '../api'
import { useAuth } from '../auth'
import { Link } from '../router'
import { defaultShardTarget, shadowmourneClasses, shadowmourneStageLabels, shadowmourneStages } from '../shadowmourne'
import { atLeast, type Character, type ShadowmourneEntry, type ShadowmourneStage } from '../types'
import { useSubmission } from '../useSubmission'

function errorMessage(err: unknown): string {
  return err instanceof ApiError ? err.message : 'Die Schattengram-Liste konnte nicht geladen werden.'
}

function shardBar(entry: ShadowmourneEntry) {
  const percent = Math.min(100, Math.round((entry.shards_collected / entry.shard_target) * 100))
  return <div className="shard-bar" title={`${entry.shards_collected} von ${entry.shard_target} Splittern`}>
    <span className="shard-bar-fill" style={{ width: `${percent}%` }} />
    <span className="shard-bar-text">{entry.shards_collected} / {entry.shard_target}</span>
  </div>
}

function stageSteps(stage: ShadowmourneStage) {
  const current = shadowmourneStages.indexOf(stage)
  return <ol className="stage-steps" aria-label={`Schritt: ${shadowmourneStageLabels[stage]}`}>
    {shadowmourneStages.map((item, index) => <li key={item} className={index < current ? 'done' : index === current ? 'current' : ''}>{shadowmourneStageLabels[item]}</li>)}
  </ol>
}

interface EditState {
  id: number
  stage: ShadowmourneStage
  shards: string
  target: string
  position: string
}

export function ShadowmournePage() {
  const { player: me } = useAuth()
  const { pending, submit, isActive } = useSubmission()
  const [entries, setEntries] = useState<ShadowmourneEntry[]>([])
  const [characters, setCharacters] = useState<Character[]>([])
  const [error, setError] = useState('')
  const [editing, setEditing] = useState<EditState | null>(null)
  const [characterID, setCharacterID] = useState('')
  const [stage, setStage] = useState<ShadowmourneStage>('PREQUEST')
  const [shards, setShards] = useState('0')
  const [target, setTarget] = useState(String(defaultShardTarget))
  const [position, setPosition] = useState('')

  const canManage = me ? atLeast(me.role, 'RAIDASSISTENT') : false
  const active = entries.filter((entry) => entry.status === 'ACTIVE')
  const completed = entries.filter((entry) => entry.status === 'COMPLETED')
  const listedCharacters = new Set(active.map((entry) => entry.character_id))
  const eligibleCharacters = characters
    .filter((character) => character.active && shadowmourneClasses.includes(character.class) && !listedCharacters.has(character.id))
    .sort((a, b) => a.name.localeCompare(b.name, 'de'))

  const load = () => {
    setError('')
    const requests: [Promise<ShadowmourneEntry[]>, Promise<Character[]>] = [
      api.get<ShadowmourneEntry[]>('/api/v1/shadowmourne'),
      canManage ? api.get<Character[]>('/api/v1/characters') : Promise.resolve([]),
    ]
    Promise.all(requests)
      .then(([list, characterRows]) => {
        if (!isActive()) return
        setEntries(list)
        setCharacters(characterRows)
      })
      .catch((err) => { if (isActive()) setError(errorMessage(err)) })
  }

  useEffect(load, [canManage, isActive])

  const write = async (work: () => Promise<unknown>) => {
    if (!canManage) return
    await submit(async () => {
      setError('')
      try {
        await work()
        if (isActive()) load()
      } catch (err) {
        if (isActive()) setError(errorMessage(err))
      }
    })
  }

  const create = async (event: FormEvent) => {
    event.preventDefault()
    await write(async () => {
      await api.post<ShadowmourneEntry>('/api/v1/shadowmourne', {
        character_id: Number(characterID),
        position: position ? Number(position) : 0,
        stage,
        shards_collected: Number(shards),
        shard_target: Number(target),
      })
      if (!isActive()) return
      setCharacterID('')
      setStage('PREQUEST')
      setShards('0')
      setTarget(String(defaultShardTarget))
      setPosition('')
    })
  }

  const saveProgress = (entry: ShadowmourneEntry, next: { stage: ShadowmourneStage, shards_collected: number, shard_target: number }) =>
    write(() => api.patch<ShadowmourneEntry>(`/api/v1/shadowmourne/${entry.id}`, next))

  const addShards = (entry: ShadowmourneEntry, delta: number) => saveProgress(entry, {
    stage: entry.stage,
    shards_collected: Math.max(0, Math.min(entry.shard_target, entry.shards_collected + delta)),
    shard_target: entry.shard_target,
  })

  const move = (entry: ShadowmourneEntry, to: number) =>
    write(() => api.post<ShadowmourneEntry[]>(`/api/v1/shadowmourne/${entry.id}/move`, { position: to }))

  const saveEdit = async (event: FormEvent) => {
    event.preventDefault()
    if (!editing) return
    const entry = active.find((item) => item.id === editing.id)
    if (!entry) return
    await write(async () => {
      await api.patch<ShadowmourneEntry>(`/api/v1/shadowmourne/${entry.id}`, { stage: editing.stage, shards_collected: Number(editing.shards), shard_target: Number(editing.target) })
      const wanted = Number(editing.position)
      if (wanted && wanted !== entry.priority) await api.post<ShadowmourneEntry[]>(`/api/v1/shadowmourne/${entry.id}/move`, { position: wanted })
      if (isActive()) setEditing(null)
    })
  }

  const complete = async (entry: ShadowmourneEntry) => {
    if (!window.confirm(`${entry.character_name} hat Schattengram fertig? Der Eintrag wandert in die Historie.`)) return
    await write(() => api.post<ShadowmourneEntry>(`/api/v1/shadowmourne/${entry.id}/complete`, {}))
  }

  const remove = async (entry: ShadowmourneEntry) => {
    if (!window.confirm(`${entry.character_name} von der Liste nehmen?`)) return
    await write(() => api.post<void>(`/api/v1/shadowmourne/${entry.id}/remove`, {}))
  }

  const startEdit = (entry: ShadowmourneEntry) => setEditing({ id: entry.id, stage: entry.stage, shards: String(entry.shards_collected), target: String(entry.shard_target), position: String(entry.priority) })

  return <div className="shadowmourne-page">
    <div className="page-heading"><div><p className="eyebrow">Questreihe & Priorität</p><h1>Schattengram</h1></div></div>
    <p className="muted">Wer an Schattengram arbeitet, in welcher Reihenfolge und bei welchem Schritt. Seuche, Blut, Frost und Splitter kann pro Raid nur eine Person machen; bei Gleichstand hat der höhere Listenplatz Vorrang. Wer im jeweiligen Raid dran ist, steht im Raid unter „Schattengram im Raid“.</p>
    {error && <p className="error" role="alert">{error}</p>}
    {pending && <p className="muted" role="status">Wird gespeichert&hellip;</p>}

    <section className="card">
      <div className="section-heading"><h2>Liste</h2><span className="badge info">{active.length} aktiv</span></div>
      {active.length === 0 && <p className="muted">Noch niemand arbeitet an Schattengram.</p>}
      <ol className="shadowmourne-list">
        {active.map((entry, index) => {
          const done = entry.shards_collected >= entry.shard_target
          const isEditing = editing?.id === entry.id
          const showShards = entry.stage === 'SHARDS' || entry.stage === 'FINAL' || entry.shards_collected > 0
          return <li key={entry.id} className="subcard shadowmourne-entry">
            <div className="shadowmourne-entry-head">
              <span className="shadowmourne-rank" aria-label={`Platz ${entry.priority}`}>{entry.priority}</span>
              <div>
                <Link to={`/characters/${entry.character_id}`}><strong>{entry.character_name}</strong></Link>
                <span className="muted"> · {entry.character_class} · {entry.player_name}</span>
              </div>
              <span className="badge ok">{shadowmourneStageLabels[entry.stage]}</span>
            </div>
            {stageSteps(entry.stage)}
            {showShards && <div className="shadowmourne-shards">
              {shardBar(entry)}
              {canManage && entry.stage === 'SHARDS' && <div className="actions">
                <button type="button" aria-label={`Ein Splitter weniger für ${entry.character_name}`} disabled={pending || entry.shards_collected === 0} onClick={() => void addShards(entry, -1)}>−1</button>
                <button type="button" aria-label={`Ein Splitter mehr für ${entry.character_name}`} disabled={pending || done} onClick={() => void addShards(entry, 1)}>+1</button>
              </div>}
            </div>}
            {canManage && !isEditing && <div className="actions">
              <button type="button" aria-label={`${entry.character_name} nach oben`} disabled={pending || index === 0} onClick={() => void move(entry, entry.priority - 1)}>↑</button>
              <button type="button" aria-label={`${entry.character_name} nach unten`} disabled={pending || index === active.length - 1} onClick={() => void move(entry, entry.priority + 1)}>↓</button>
              <button type="button" disabled={pending} onClick={() => startEdit(entry)}>Bearbeiten</button>
              <button type="button" disabled={pending || !done} title={done ? undefined : 'Erst wenn alle Splitter gesammelt sind'} onClick={() => void complete(entry)}>Fertig</button>
              <button type="button" disabled={pending} onClick={() => void remove(entry)}>Entfernen</button>
              {!done && entry.stage === 'FINAL' && <span className="muted">„Fertig“ geht, sobald alle Splitter gesammelt sind.</span>}
            </div>}
            {canManage && isEditing && editing && <form className="actions shadowmourne-edit" onSubmit={saveEdit}>
              <label>Schritt<select aria-label="Schritt bearbeiten" value={editing.stage} onChange={(event) => setEditing({ ...editing, stage: event.target.value as ShadowmourneStage })}>{shadowmourneStages.map((item) => <option key={item} value={item}>{shadowmourneStageLabels[item]}</option>)}</select></label>
              <label>Splitter<input aria-label="Splitter bearbeiten" type="number" min="0" max={editing.target} value={editing.shards} onChange={(event) => setEditing({ ...editing, shards: event.target.value })} required /></label>
              <label>Ziel<input aria-label="Ziel bearbeiten" type="number" min="1" value={editing.target} onChange={(event) => setEditing({ ...editing, target: event.target.value })} required /></label>
              <label>Platz<input aria-label="Platz bearbeiten" type="number" min="1" max={active.length} value={editing.position} onChange={(event) => setEditing({ ...editing, position: event.target.value })} required /></label>
              <button className="primary-action" type="submit" disabled={pending}>Speichern</button>
              <button type="button" disabled={pending} onClick={() => setEditing(null)}>Abbrechen</button>
            </form>}
          </li>
        })}
      </ol>
    </section>

    {canManage && <details className="card editor-disclosure"><summary>Charakter eintragen</summary>
      <form className="actions shadowmourne-edit" onSubmit={create}>
        <label>Charakter<select aria-label="Schattengram-Charakter" value={characterID} onChange={(event) => setCharacterID(event.target.value)} required><option value="">Bitte wählen</option>{eligibleCharacters.map((character) => <option key={character.id} value={character.id}>{character.name} ({character.class})</option>)}</select></label>
        <label>Schritt<select aria-label="Neuer Schritt" value={stage} onChange={(event) => setStage(event.target.value as ShadowmourneStage)}>{shadowmourneStages.map((item) => <option key={item} value={item}>{shadowmourneStageLabels[item]}</option>)}</select></label>
        <label>Splitter<input aria-label="Neue Splitter" type="number" min="0" value={shards} onChange={(event) => setShards(event.target.value)} required /></label>
        <label>Ziel<input aria-label="Neues Splitterziel" type="number" min="1" value={target} onChange={(event) => setTarget(event.target.value)} required /></label>
        <label>Platz<input aria-label="Neuer Platz" type="number" min="1" placeholder="Ende" value={position} onChange={(event) => setPosition(event.target.value)} /></label>
        <button className="primary-action" type="submit" disabled={pending}>Eintragen</button>
      </form>
      <p className="muted">Nur Krieger, Paladine und Todesritter. Ohne Platz kommt der Charakter ans Ende der Liste.</p>
    </details>}

    <section className="card">
      <div className="section-heading"><h2>Fertig</h2><span className="eyebrow">Historie</span></div>
      <div className="table-scroll"><table className="table"><thead><tr><th>Charakter</th><th>Spieler</th><th>Fertig am</th></tr></thead><tbody>
        {completed.map((entry) => <tr key={entry.id}><td><Link to={`/characters/${entry.character_id}`}>{entry.character_name}</Link></td><td>{entry.player_name}</td><td>{entry.completed_at ? new Date(entry.completed_at).toLocaleDateString('de-DE') : '-'}</td></tr>)}
        {completed.length === 0 && <tr><td colSpan={3} className="muted">Noch niemand hat Schattengram fertig.</td></tr>}
      </tbody></table></div>
    </section>
  </div>
}
