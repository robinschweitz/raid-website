import { characterClassName } from '../characterTheme'
import type { Attendance } from '../types'

interface Props {
  attendance: Attendance[]
  playerName: (id: number) => string
  canWrite: boolean
  pending: boolean
  onEdit: (row: Attendance) => void
}

const groups = [
  { role: 'TANK', label: 'Tanks', symbol: '◇' },
  { role: 'HEAL', label: 'Heilung', symbol: '+' },
  { role: 'DD', label: 'Schaden', symbol: '⚔' },
] as const

export function RaidRosterBoard({ attendance, playerName, canWrite, pending, onEdit }: Props) {
  const member = (row: Attendance) => (
    <li key={row.id} className={`setup-member ${characterClassName(row.class ?? '')}`}>
      <span className="setup-sigil" aria-hidden="true">{row.character_name?.trim().slice(0, 1) || '?'}</span>
      <div className="setup-member-info">
        <strong className="character-name">{row.character_name?.trim() || 'Name nicht gespeichert'}</strong>
        <span>{row.spec?.trim() || 'Spec unbekannt'} · {row.class?.trim() || 'Klasse unbekannt'}</span>
        <small>{playerName(row.player_id)}</small>
        <div className="setup-flags">
          {row.is_bench && <span className="badge">{groups.find((group) => group.role === row.role)?.label ?? row.role}</span>}
          {row.is_substitute && <span className="badge">Ersatz</span>}
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
    </li>
  )
  const bench = attendance.filter((row) => row.is_bench)
  return <>
    <div className="setup-board">
      {groups.map(({ role, label, symbol }) => {
        const rows = attendance.filter((row) => row.role === role && !row.is_bench)
        return <section className={`setup-column setup-${role.toLowerCase()}`} key={role} aria-label={label}>
          <header><h3><span aria-hidden="true">{symbol}</span> {label}</h3><span className="badge">{rows.length}</span></header>
          <ul>{rows.map(member)}</ul>
          {rows.length === 0 && <p className="setup-empty">Noch nicht besetzt</p>}
        </section>
      })}
    </div>
    <section className="setup-bench" aria-label="Bank">
      <header><h3>Bank</h3><span className="badge">{bench.length}</span></header>
      {bench.length ? <ul>{bench.map(member)}</ul> : <p className="muted">Keine Teilnehmer auf der Bank.</p>}
    </section>
  </>
}
