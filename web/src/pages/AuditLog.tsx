import { useEffect, useState } from 'react'
import { api, ApiError } from '../api'
import { useAuth } from '../auth'
import { Link } from '../router'
import { atLeast, type AuditLogEntry } from '../types'

type AuditSnapshot = Record<string, unknown> | null

interface AuditChange {
  label: string
  before?: unknown
  after?: unknown
}

const fieldLabels: Record<string, string> = {
  active: 'Aktiv', award_method: 'Vergabemethode', awarded_to_character_name: 'Empfänger', boss: 'Boss',
  character_name: 'Charakter', class: 'Klasse', enhanced_cost: 'Enhance-Kosten', gear_score: 'Gegenstandswert',
  is_bench: 'Ersatzbank', is_main: 'Hauptcharakter', is_substitute: 'Ersatzspieler', late_join: 'Später Einstieg',
  early_leave: 'Früher Ausstieg', main_spec: 'Hauptspezialisierung', name: 'Name', notes: 'Notiz',
  role: 'Rolle', second_spec: 'Zweitspezialisierung', setup: 'Soll-Aufstellung', status: 'Status',
  text: 'Gesuchtext', without_points: 'Keine EP', is_enabled: 'Anmeldungen aktiviert', item_name: 'Gegenstand',
  withdrawn_at: 'Zurückgezogen', withdrawn_reason: 'Grund für Rücknahme', spec: 'Spezialisierung',
}

const hiddenFields = new Set([
  'id', 'player_id', 'character_id', 'awarded_to_player_id', 'awarded_to_character_id', 'awarded_by_player_id',
  'raid_id', 'created_at', 'updated_at', 'name_changed_at', 'external_source', 'external_profile_id',
  'external_profile_url', 'catalog_boss_id', 'item_id', 'item_lock_id', 'request_id', 'source',
])

function parseSnapshot(snapshot: string): AuditSnapshot {
  try {
    const value: unknown = JSON.parse(snapshot)
    return value !== null && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : null
  } catch {
    return null
  }
}

function displaySnapshot(snapshot: string): string {
  if (!snapshot) return 'Keine Daten'
  try {
    return JSON.stringify(JSON.parse(snapshot), null, 2)
  } catch {
    return snapshot
  }
}

function textValue(snapshot: AuditSnapshot, field: string): string {
  const value = snapshot?.[field]
  return typeof value === 'string' ? value : ''
}

function formatValue(value: unknown): string {
  if (value === null || value === undefined || value === '') return 'Nicht gesetzt'
  if (typeof value === 'boolean') return value ? 'Ja' : 'Nein'
  if (value === 'DD') return 'Schaden'
  if (value === 'HEAL') return 'Heiler'
  if (value === 'TANK') return 'Tank'
  if (value === 'MAIN') return 'Main-Spec'
  if (value === 'SECOND') return 'Second-Spec'
  if (value === 'ENHANCED') return 'Enhance'
  if (value === 'BOE') return 'BoE'
  if (Array.isArray(value)) return value.map(formatValue).join(', ') || 'Keine'
  if (typeof value === 'object') return JSON.stringify(value)
  return String(value)
}

function hasMeaningfulValue(field: string, value: unknown): boolean {
  if (value === null || value === undefined || value === '' || value === 0) return false
  if (typeof value === 'boolean') return value && !['active', 'is_enabled'].includes(field)
  return true
}

function hasChanged(before: unknown, after: unknown): boolean {
  return JSON.stringify(before) !== JSON.stringify(after)
}

function changesFor(before: AuditSnapshot, after: AuditSnapshot): AuditChange[] {
  if (!before && !after) return []
  const fields = [...new Set([...Object.keys(before ?? {}), ...Object.keys(after ?? {})])]
    .filter((field) => !hiddenFields.has(field))
    .sort((a, b) => (fieldLabels[a] ?? a).localeCompare(fieldLabels[b] ?? b, 'de'))
  if (!before) return fields.flatMap((field) => hasMeaningfulValue(field, after?.[field]) ? [{ label: fieldLabels[field] ?? field, after: after?.[field] }] : [])
  if (!after) return fields.flatMap((field) => hasMeaningfulValue(field, before[field]) ? [{ label: fieldLabels[field] ?? field, before: before[field] }] : [])
  return fields.flatMap((field) => hasChanged(before[field], after[field]) ? [{ label: fieldLabels[field] ?? field, before: before[field], after: after[field] }] : [])
}

function actionLabel(entry: AuditLogEntry, before: AuditSnapshot): string {
  if (entry.action === 'correction') return before ? 'Korrigiert' : 'Hinzugefügt'
  const labels: Record<string, string> = {
    accept: 'Bestätigt', absence_declare: 'Abgesagt', absence_revoke: 'Absage zurückgenommen',
    archive_without_points: 'EP-Ausnahme festgelegt', availability: 'Anmeldungen geändert', award: 'Beute vergeben',
    create_linked_character: 'Charakter angelegt', link_rising_gods_profile: 'Rising-Gods-Profil verknüpft',
    merge_duplicate_source: 'Duplikat zusammengeführt', merge_duplicate_target: 'Duplikat übernommen',
    reload_rising_gods_profile: 'Rising-Gods-Profil aktualisiert', rename: 'Umbenannt', remove: 'Aus dem Raid entfernt', save: 'Gespeichert',
    submit: 'Angemeldet', update: 'Aktualisiert', withdraw: 'Zurückgezogen', claim_character: 'Charakter übernommen',
  }
  return labels[entry.action] ?? entry.action.replace(/_/g, ' ')
}

function entityDescription(entry: AuditLogEntry, before: AuditSnapshot, after: AuditSnapshot): string {
  const snapshot = after ?? before
  const name = textValue(snapshot, 'character_name') || textValue(snapshot, 'name')
  const raidID = snapshot?.raid_id
  const raid = typeof raidID === 'number' ? ` in Raid #${raidID}` : ''
  switch (entry.entity_type) {
    case 'attendance':
    case 'raid_quick_add':
      return `Teilnahme: ${name || 'Unbekannter Charakter'}${raid}`
    case 'character': return `Charakter: ${name || `#${entry.entity_id}`}`
    case 'loot_award': {
      const recipient = textValue(snapshot, 'awarded_to_character_name')
      return `Beute: ${textValue(snapshot, 'item_name') || 'Unbekannter Gegenstand'}${recipient ? ` an ${recipient}` : ''}${raid}`
    }
    case 'player': return `Spieler: ${name || `#${entry.entity_id}`}`
    case 'raid': return `Raid #${entry.entity_id}`
    case 'raid_signup': return `Raid-Anmeldung für Raid #${entry.entity_id}`
    case 'raid_recruitment_template': return 'Raid-Gesuchtext'
    default: return `${entry.entity_type} #${entry.entity_id}`
  }
}

function entityTarget(entry: AuditLogEntry, before: AuditSnapshot, after: AuditSnapshot): string | null {
  if (entry.entity_type === 'character') return `/characters/${entry.entity_id}`
  if (entry.entity_type === 'player') return `/players/${entry.entity_id}`
  if (entry.entity_type === 'raid' || entry.entity_type === 'raid_signup') return `/raids/${entry.entity_id}`
  const raidID = (after ?? before)?.raid_id
  return typeof raidID === 'number' ? `/raids/${raidID}` : null
}

export function AuditLogPage() {
  const { player } = useAuth()
  const [entries, setEntries] = useState<AuditLogEntry[]>([])
  const [query, setQuery] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const isAdmin = !!player && atLeast(player.role, 'ADMIN')

  useEffect(() => {
    if (!isAdmin) {
      setLoading(false)
      return
    }
    let active = true
    setLoading(true)
    setError('')
    api.get<AuditLogEntry[]>('/api/v1/audit-log')
      .then((rows) => { if (active) setEntries(rows) })
      .catch((err) => { if (active) setError(err instanceof ApiError ? err.message : 'Audit-Protokoll konnte nicht geladen werden.') })
      .finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [isAdmin])

  const normalizedQuery = query.trim().toLocaleLowerCase('de-DE')
  const visibleEntries = normalizedQuery
    ? entries.filter((entry) => [entry.actor_name, entry.actor_id, entry.entity_type, entry.entity_id, entry.action, entry.reason, entry.before_json, entry.after_json]
      .some((value) => String(value ?? '').toLocaleLowerCase('de-DE').includes(normalizedQuery)))
    : entries

  if (player && !isAdmin) return <div className="audit-log">
    <p><Link to="/">← Übersicht</Link></p>
    <div className="page-heading"><div><p className="eyebrow">Administration</p><h1>Audit-Protokoll</h1></div></div>
    <p className="muted" role="status">Dieser Bereich ist nur für Admins sichtbar.</p>
  </div>

  return <div className="audit-log">
    <p><Link to="/roster">← Gildenkonfiguration</Link></p>
    <div className="page-heading"><div><p className="eyebrow">Administration</p><h1>Audit-Protokoll</h1></div></div>
    <p className="muted">Änderungen sind lesbar zusammengefasst. Die technischen Rohdaten bleiben in den Details für die Fehlersuche verfügbar.</p>
    {error && <p className="error" role="alert">{error}</p>}
    <label className="audit-search">Protokoll durchsuchen<input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Akteur, Aktion, Objekt oder Grund" /></label>
    <p className="muted" role="status">{visibleEntries.length} von {entries.length} Einträgen</p>
    {visibleEntries.length > 0 && <div className="audit-table-wrap"><table className="table audit-table">
      <thead><tr><th>Zeitpunkt</th><th>Akteur</th><th>Ereignis</th><th>Änderung</th><th>Grund</th><th>Details</th></tr></thead>
      <tbody>{visibleEntries.map((entry) => {
        const before = parseSnapshot(entry.before_json)
        const after = parseSnapshot(entry.after_json)
        const target = entityTarget(entry, before, after)
        const description = entityDescription(entry, before, after)
        const changes = changesFor(before, after)
        return <tr key={entry.id}>
          <td><time dateTime={entry.created_at}>{new Date(entry.created_at).toLocaleString('de-DE')}</time></td>
          <td>{entry.actor_name ?? (entry.actor_id == null ? 'System' : `Gelöschter Spieler #${entry.actor_id}`)}</td>
          <td className="audit-event">{target ? <Link to={target}>{description}</Link> : description}</td>
          <td><span className="audit-action">{actionLabel(entry, before)}</span></td>
          <td>{entry.reason || '-'}</td>
          <td><details className="audit-details"><summary>Änderung ansehen</summary><div>
            {changes.length > 0 ? <dl className="audit-change-list">{changes.map((change) => <div key={change.label}>
              <dt>{change.label}</dt><dd>{change.before !== undefined && <span className="audit-before">{formatValue(change.before)}</span>}{change.before !== undefined && change.after !== undefined && <span className="audit-arrow">→</span>}{change.after !== undefined && <span className="audit-after">{formatValue(change.after)}</span>}{change.after === undefined && <span className="audit-after">Entfernt</span>}</dd>
            </div>)}</dl> : <p className="muted">Keine fachlich relevanten Felder in diesem Eintrag.</p>}
            <details className="audit-raw-data"><summary>Technische Rohdaten</summary><strong>Vorher</strong><pre>{displaySnapshot(entry.before_json)}</pre><strong>Nachher</strong><pre>{displaySnapshot(entry.after_json)}</pre></details>
          </div></details></td>
        </tr>
      })}</tbody>
    </table></div>}
    {loading && <p className="muted" role="status">Audit-Protokoll wird geladen...</p>}
    {!loading && !error && visibleEntries.length === 0 && <p className="muted">{entries.length === 0 ? 'Noch keine Audit-Einträge vorhanden.' : 'Keine Einträge für diese Suche.'}</p>}
  </div>
}
