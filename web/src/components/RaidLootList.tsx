import type { ReactNode } from 'react'
import type { Attendance, LootAward } from '../types'
import { characterClassName } from '../characterTheme'
import { Link } from '../router'

type RecordedLoot = LootAward & { award_method?: 'MAIN' | 'SECOND' | 'ENHANCED' | 'BOE'; enhanced_cost?: number | null }
const labels = { MAIN: 'Main-Spec', SECOND: 'Second-Spec', ENHANCED: 'Enhanced', BOE: 'BoE' }
const methodClasses = { MAIN: 'main', SECOND: 'second', ENHANCED: 'enhanced', BOE: 'boe' }

interface Props {
  loot: RecordedLoot[]
  attendance: Attendance[]
  playerName: (id: number) => string
  canWrite: boolean
  pending: boolean
  editingID?: number
  onEdit: (row: RecordedLoot) => void
  children: ReactNode
}

export function RaidLootList({ loot, attendance, playerName, canWrite, pending, editingID, onEdit, children }: Props) {
  const bossGroups = [...new Set(loot.map((row) => row.boss))].map((boss) => ({
    boss,
    rows: loot.filter((row) => row.boss === boss),
  }))

  return <>
    {canWrite && editingID == null && <details className="editor-disclosure"><summary>Loot erfassen</summary>{children}</details>}
    {loot.length === 0 && <p className="muted">Kein Loot.</p>}
    {bossGroups.map(({ boss, rows }) => <div key={boss} className="subcard loot-boss-group">
      <div className="loot-boss-heading"><h3>{boss || 'Boss nicht gespeichert (historisch)'}</h3><span>{rows.length} Vergabe{rows.length === 1 ? '' : 'n'}</span></div>
      <div className="table-scroll"><table className="table loot-table">
        <thead><tr><th>Item</th><th>Boss</th><th>Empfänger</th><th>Vergabemethode</th><th>EP-Kosten</th><th>Notizen</th>{canWrite && <th></th>}</tr></thead>
        <tbody>{rows.map((row) => {
          const saved = attendance.find((a) => a.character_id === row.awarded_to_character_id && a.player_id === row.awarded_to_player_id)
          const method = row.award_method ?? (row.is_main_spec ? 'MAIN' : 'SECOND')
          return <tr key={row.id}>
            {editingID === row.id ? <td colSpan={7}><div className="in-place-editor"><h3>{row.item_name} bearbeiten</h3>
              {method === 'ENHANCED' && <p className="muted">Gespeicherte Kosten: {row.enhanced_cost == null ? 'Historisch nicht gespeichert' : `${row.enhanced_cost} EP`}</p>}
              {children}</div></td> : <>
              <td><strong className="loot-item-name">{row.item_name}</strong></td><td>{row.boss || '—'}</td>
              <td><div className="loot-recipient"><span className={`character-name ${characterClassName(saved?.class ?? '')}`}>{row.awarded_to_character_id ? <Link to={`/characters/${row.awarded_to_character_id}`}>{row.awarded_to_character_name?.trim() || saved?.character_name?.trim() || 'Charakterprofil'}</Link> : row.awarded_to_character_name?.trim() || saved?.character_name?.trim() || 'Charaktername historisch nicht gespeichert'}</span><span>{playerName(row.awarded_to_player_id)}</span></div></td>
              <td><span className={`loot-method ${methodClasses[method]}`}>{labels[method]}</span>{row.item_lock_id && <span className="badge">Lock</span>}</td>
              <td><span className="loot-cost">{method === 'ENHANCED' ? row.enhanced_cost == null ? 'Historisch nicht gespeichert' : `${row.enhanced_cost} EP` : '0 EP'}</span></td>
              <td>{row.notes || '—'}</td>
              {canWrite && <td><button disabled={pending} onClick={() => onEdit(row)}>Loot korrigieren</button></td>}
            </>}
          </tr>
        })}</tbody>
      </table></div>
    </div>)}
  </>
}
