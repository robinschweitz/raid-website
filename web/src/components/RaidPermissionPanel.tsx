import { useEffect, useState } from 'react'
import { api } from '../api'
import { ErrorMessage } from './ErrorMessage'
import { useSubmission } from '../useSubmission'

const actions = [[1, 'Anmeldungen und Aufstellung'], [2, 'Private Absagenotizen'], [4, 'Loot und EP'], [8, 'Planung und Abschluss']] as const
const sources: Record<string, string> = { admin: 'Administrator', role: 'Kontorolle', policy: 'Vom Admin festgelegt', outside_scope: 'Nicht für diesen Termin eingeteilt', substitute: 'Vertretung für diesen Termin' }
type Effective = { allowed: number; source: string }
type Policy = { allowed: number; assigned_only: boolean }
type Overview = { rights: Effective; delegable: number; substitutes: { player_id: number; grantor_id: number; allowed: number }[]; team: { id: number; name: string; role: string; effective: Effective }[] }
const describe = (bits: number) => actions.filter(([bit]) => bits & bit).map(([, label]) => label).join(', ') || 'Keine'
const explain = (err: unknown) => err instanceof Error ? err.message : 'Berechtigungen konnten nicht geladen werden.'

export function PlayerRaidRights({playerID}: {playerID: number}) {
  const [open, setOpen] = useState(false), [archived, setArchived] = useState(false)
  const [rows,setRows] = useState<{id:number; name:string; rights: Effective}[]>([]), [error,setError] = useState('')
  useEffect(() => {let live=true; if(open) api.get<typeof rows>(`/api/v1/players/${playerID}/raid-permissions/overview?include_archived=${archived}`).then(v=>{if(live){setRows(v);setError('')}}).catch(err=>{if(live){setRows([]);setError(explain(err))}});return ()=>{live=false}},[open,archived,playerID])
  return <details onToggle={e=>setOpen(e.currentTarget.open)}><summary>Berechtigungen nach Raid</summary>{open && <><label className="check"><input type="checkbox" checked={archived} onChange={e=>setArchived(e.target.checked)}/> Archivierte Raids anzeigen</label>{error && <ErrorMessage>{error}</ErrorMessage>}<ul>{rows.map(r=><li key={r.id}><a href={`#/raids/${r.id}`}>{r.name}</a>: {describe(r.rights.allowed)} · {sources[r.rights.source] ?? 'Inaktives Konto'}</li>)}</ul></>}</details>
}

function Checks({ value, ceiling, onChange }: { value: number; ceiling: number; onChange: (v: number) => void }) {
  return <>{actions.map(([bit, label]) => <label key={bit} className="check"><input type="checkbox" checked={!!(value & bit)} disabled={!(ceiling & bit)} onChange={e => onChange(e.target.checked ? value | bit : value & ~bit)} /> {label}</label>)}</>
}

export function RaidPermissionPanel(props: { raidID: number; admin: boolean; onRefresh: () => Promise<void> }) {
  const [open, setOpen] = useState(false)
  return <details onToggle={e => setOpen(e.currentTarget.open)}><summary>Berechtigungen und Vertretung</summary>{open && <Manager {...props} />}</details>
}

function Manager({ raidID, admin, onRefresh }: { raidID: number; admin: boolean; onRefresh: () => Promise<void> }) {
  const [data, setData] = useState<Overview | null>(null)
  const [error, setError] = useState('')
  const [target, setTarget] = useState(0)
  const [version, setVersion] = useState(0)
  useEffect(() => { let live = true; api.get<Overview>(`/api/v1/raids/${raidID}/permissions`).then(v => { if (live) { setData(v); setError('') } }).catch(err => { if (live) setError(explain(err)) }); return () => { live = false } }, [raidID, version])
  return <section>{error && <ErrorMessage>{error}</ErrorMessage>}{data && <>
    <p>Deine Rechte in diesem Raid: <strong>{describe(data.rights.allowed)}</strong> · {sources[data.rights.source]}</p>
    {!!data.team.length && <><ul>{data.team.map(p => <li key={p.id}><strong>{p.name}</strong>: {describe(p.effective.allowed)} · {sources[p.effective.source]}</li>)}</ul>
      <label>Teammitglied <select value={target} onChange={e => setTarget(Number(e.target.value))}><option value={0}>Bitte wählen</option>{data.team.map(p => <option key={p.id} value={p.id}>{p.name}</option>)}</select></label>
      {!!target && <Editor key={`${raidID}:${target}`} raidID={raidID} target={target} admin={admin} data={data} afterSave={async () => {setVersion(v => v + 1); await onRefresh()}} />}
    </>}
  </>}</section>
}

function Editor({ raidID, target, admin, data, afterSave }: { raidID: number; target: number; admin: boolean; data: Overview; afterSave: () => Promise<void> }) {
  const [policy, setPolicy] = useState<Policy | null>(null)
  const [bits, setBits] = useState(data.substitutes.find(s => s.player_id === target)?.allowed ?? 0)
  const [error, setError] = useState(''), [message, setMessage] = useState('')
  const { pending, submit, isActive } = useSubmission()
  const path = `/api/v1/players/${target}/raid-permissions`
  useEffect(() => { let live = true; if (admin) api.get<Policy>(path).then(v => { if (live) setPolicy(v) }).catch(err => { if (live) setError(explain(err)) }); return () => { live = false } }, [admin, path])
  function save(work: () => Promise<unknown>) { void submit(async () => { setError(''); setMessage(''); try { await work(); if (isActive()) { await afterSave(); setMessage('Berechtigungen gespeichert.') } } catch (err) { if (isActive()) setError(explain(err)) } }) }
  return <>{error && <ErrorMessage>{error}</ErrorMessage>}{message && <p role="status">{message}</p>}
    {admin && policy && <fieldset disabled={pending}><legend>Kontorechte für alle Raids</legend>
      <label>Geltungsbereich <select value={policy.assigned_only ? 'assigned' : 'all'} onChange={e => setPolicy({ ...policy, assigned_only: e.target.value === 'assigned' })}><option value="all">Alle Raids</option><option value="assigned">Nur eigene Termine und Vertretungen</option></select></label>
      <Checks value={policy.allowed} ceiling={15} onChange={allowed => setPolicy({ ...policy, allowed })} /><button type="button" onClick={() => save(() => api.put(path, policy))}>Kontorechte speichern</button>
    </fieldset>}
    {!!data.delegable && <fieldset disabled={pending}><legend>Vertretung nur für diesen Raid</legend>
      <p>Die Vertretung erweitert den Geltungsbereich für diesen Termin. Die vom Admin erlaubten Aufgaben bleiben die Obergrenze.</p>
      <Checks value={bits} ceiling={data.delegable} onChange={setBits} /><button type="button" disabled={!bits} onClick={() => save(() => api.put(`/api/v1/raids/${raidID}/substitutes/${target}`, { allowed: bits }))}>Vertretung speichern</button>
      {data.substitutes.some(s => s.player_id === target) && <button type="button" onClick={() => save(async () => { await api.put(`/api/v1/raids/${raidID}/substitutes/${target}`, { allowed: 0 }); setBits(0) })}>Vertretung aufheben</button>}
    </fieldset>}
  </>
}
