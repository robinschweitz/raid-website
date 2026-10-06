import { characterClassName } from '../characterTheme'
import { rosterGroup, rosterGroups } from '../raidRoster'
import { ClassEmblem } from './ClassEmblem'
import type { Attendance } from '../types'
import type { ReactNode } from 'react'

interface Props {
  attendance: Attendance[]
  playerName: (id: number) => string
  canWrite: boolean
  pending: boolean
  onEdit: (row: Attendance) => void
  editingID?: number
  children?: ReactNode
}

export function RaidRosterBoard({ attendance, playerName, canWrite, pending, onEdit, editingID, children }: Props) {
  const member = (row: Attendance) => (
    <li key={row.id} className={`setup-member ${characterClassName(row.class?.trim() ?? '')}${editingID === row.id ? ' is-editing' : ''}`}>
      {editingID === row.id ? <div className="in-place-editor"><h3>{row.character_name || 'Teilnahme'} bearbeiten</h3>{children}</div> : <>
      <span className="setup-sigil"><ClassEmblem characterClass={row.class ?? ''} /></span>
      <div className="setup-member-info">
        <strong className="character-name">{row.character_name?.trim() || 'Name nicht gespeichert'}</strong>
        <span>{row.spec?.trim() || 'Spec unbekannt'} · {row.class?.trim() || 'Klasse unbekannt'}</span>
        <small>{playerName(row.player_id)}</small>
        <div className="setup-flags">
          {row.is_bench && <span className="badge">{rosterGroups.find((group) => group.role === rosterGroup(row))?.label}</span>}
          {row.late_join && <span className="badge">Später dabei</span>}
          {row.early_leave && <span className="badge">Früher gegangen</span>}
        </div>
        {row.notes && <p className="setup-note">{row.notes}</p>}
        {canWrite && (!row.character_name?.trim() || !row.class?.trim() || !row.spec?.trim()) && <small className="error">Teilnahme #{row.id}: {[
          !row.character_name?.trim() && 'Charaktername', !row.class?.trim() && 'Klasse', !row.spec?.trim() && 'tatsächlicher Spec',
        ].filter(Boolean).join(', ')} fehlt.</small>}
        {canWrite && <button type="button" className="setup-edit" title="Teilnahme korrigieren" disabled={pending} onClick={() => onEdit(row)}>
          <span className="visually-hidden">Teilnahme korrigieren</span>
        </button>}
      </div>
      </>}
    </li>
  )
  const bench = attendance.filter((row) => row.is_bench)
  return <>
    <div className="setup-board">
      {rosterGroups.map(({ role, label, symbol }) => {
        const rows = attendance.filter((row) => rosterGroup(row) === role && !row.is_bench)
        if (role === 'UNKNOWN' && rows.length === 0) return null
        return <section className={`setup-column setup-${role.toLowerCase()}`} key={role} aria-label={label}>
          <header><h3><span aria-hidden="true">{symbol}</span> {label}</h3><span className="badge">{rows.length}</span></header>
          {role === 'UNKNOWN' && <p className="setup-group-hint">Klasse oder Spec nicht eindeutig zuordenbar.</p>}
          <ul>{rows.map(member)}</ul>
          {rows.length === 0 && <p className="setup-empty">Noch nicht besetzt</p>}
        </section>
      })}
    </div>
    <section className="setup-bench" aria-label="Ersatzbank">
      <header><h3>Ersatzbank</h3><span className="badge">{bench.length}</span></header>
      {bench.length ? <ul>{bench.map(member)}</ul> : <p className="muted">Keine Teilnehmer auf der Ersatzbank.</p>}
    </section>
    {canWrite && editingID == null && <details className="editor-disclosure"><summary>Teilnahme hinzufügen</summary>{children}</details>}
  </>
}
