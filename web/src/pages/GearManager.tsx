import { useEffect, useState } from 'react'
import { api, ApiError } from '../api'
import { useAuth } from '../auth'
import { canonicalClass } from '../characterOptions'
import { Link } from '../router'
import { atLeast, type Character, type ExternalCharacter, type WishlistWish } from '../types'
import { ErrorMessage } from '../components/ErrorMessage'

type Setup = { character: Character; profileRace: string; wishes: string[]; addWish?: (id: string, name: string) => Promise<void>; removeWish: (id: string) => Promise<void> }
type PlannerWindow = Window & { startGearPlanner?: (setup: Setup) => void }

function Planner({character, canWish}: {character: Character; canWish: boolean}) {
  const [wishes, setWishes] = useState<string[] | null>(null)
  const [error, setError] = useState('')
  const [race, setRace] = useState<string | null>(null)
  useEffect(() => {
    let active = true
    const id = character.external_profile_id
    if (character.external_source === 'rising-gods' && id) {
      const ref = `https://db.rising-gods.de/?profile=load&id=${encodeURIComponent(id)}`
      api.get<ExternalCharacter[]>(`/api/v1/external/rising-gods/characters?q=${encodeURIComponent(ref)}`)
        .then(rows => { if (active) setRace(rows.find(row => row.profile_id === id)?.race ?? '') })
        .catch(() => { if (active) setRace('') })
    } else setRace('')
    return () => { active = false }
  }, [character.external_profile_id, character.external_source])
  useEffect(() => {
    let active = true
    if (canWish) api.get<WishlistWish[]>(`/api/v1/characters/${character.id}/wishlist`)
      .then(rows => { if (active) setWishes(rows.map(row => row.item_id)) })
      .catch(() => { if (active) {setError('Wunschliste konnte nicht geladen werden.'); setWishes([])} })
    else setWishes([])
    return () => { active = false }
  }, [character.id, canWish])
  const addWish = async (id: string, name: string) => {
    try { await api.post(`/api/v1/characters/${character.id}/wishlist/items`, {item_id:id, item_name:name, priority:0, notes:''}) }
    catch (e) { if (!(e instanceof ApiError && e.code === 'wishlist_conflict')) throw e }
  }
  return <>{error && <ErrorMessage>{error}</ErrorMessage>}{race === null && <p className="muted">Lade Volk aus dem Charakterprofil…</p>}{race !== null && wishes !== null && <iframe title="BiS-Ausrüstung und 3D-Anprobe" className="gear-planner-frame" src="/api/v1/gear-manager/" onLoad={e => {
    const frame = e.currentTarget
    const win = frame.contentWindow as PlannerWindow | null
    if (!win?.startGearPlanner) { setError('Der Gear-Manager ist nicht verfügbar. Bitte die Seite neu laden oder die Gildenleitung informieren.'); return }
    win.startGearPlanner({character: {...character, class: canonicalClass(character.class)}, profileRace: race, wishes, addWish: canWish && !error ? addWish : undefined,
      removeWish: (id) => api.del<void>(`/api/v1/characters/${character.id}/wishlist/items/${encodeURIComponent(id)}`)})
  }} />}</>
}

export function GearManagerPage({initialID}: {initialID: number}) {
  const {player} = useAuth()
  const [chars, setChars] = useState<Character[] | null>(null)
  const [selected, select] = useState(0)
  const [error, setError] = useState('')
  useEffect(() => {
    if (!player) return
    let active = true
    api.get<Character[]>(`/api/v1/players/${player.id}/characters`).then(rows => {
      if (!active) return
      const own = rows.filter(c => c.active && c.player_id === player.id).sort((a,b) => Number(b.is_main)-Number(a.is_main) || a.name.localeCompare(b.name,'de'))
      setChars(own); select((own.find(c => c.id === initialID) ?? own[0])?.id ?? 0)
    }).catch(() => {if (active) setError('Deine Charaktere konnten nicht geladen werden.')})
    return () => {active = false}
  }, [player, initialID])
  const char = chars?.find(c => c.id === selected)
  return <><div className="page-heading"><div><p className="eyebrow">Ausrüstung</p><h1>BiS-Gear-Manager</h1></div></div>
    {error && <ErrorMessage>{error}</ErrorMessage>}
    {!error && !chars && <p>Lade deine Charaktere…</p>}
    {chars?.length === 0 && <p className="card">Noch keine aktiven Charaktere. <Link to={`/players/${player?.id}`}>Zum Spielerprofil</Link></p>}
    {char && player && <><section className="card"><label>Mein Charakter <select value={char.id} onChange={e => select(Number(e.target.value))}>{chars?.map(c => <option key={c.id} value={c.id}>{c.name} · {c.class}{c.is_main ? ' · Main' : ''}</option>)}</select></label>
      <p className="muted">{char.main_spec} · <Link to={`/characters/${char.id}`}>Charakterprofil und Wunschliste</Link></p></section>
      <Planner key={char.id} character={char} canWish={atLeast(player.role,'MEMBER')} /></>}
  </>
}
