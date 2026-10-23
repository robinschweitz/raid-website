import { useEffect, useState } from 'react'
import { Bar, BarChart, Cell, Legend, Pie, PieChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { api, ApiError } from '../api'
import { Link } from '../router'
import type { Attendance, Character, LootAward, Player, Raid } from '../types'
import { ErrorMessage } from '../components/ErrorMessage'

type RaidDetails = Record<number, { attendance: Attendance[]; loot: LootAward[] }>

const COLORS = ['#86d9f2', '#8bd4b0', '#ff969c', '#c47dff', '#ffd58a', '#7f93a5', '#b4a4ec', '#ed9da9']

const CLASS_LABELS: Record<string, string> = {
  DEATHKNIGHT: 'Todesritter',
  DRUID: 'Druide',
  HUNTER: 'Jaeger',
  MAGE: 'Magier',
  PALADIN: 'Paladin',
  PRIEST: 'Priester',
  ROGUE: 'Schurke',
  SHAMAN: 'Schamane',
  WARLOCK: 'Hexenmeister',
  WARRIOR: 'Krieger',
}

const METHOD_LABELS: Record<string, string> = {
  MAIN: 'Main',
  SECOND: 'Second',
  ENHANCED: 'Enhanced',
  BOE: 'BoE',
}

function countBy<T>(rows: T[], key: (row: T) => string): { name: string; value: number }[] {
  const counts = new Map<string, number>()
  rows.forEach((row) => {
    const name = key(row).trim() || 'Unbekannt'
    counts.set(name, (counts.get(name) ?? 0) + 1)
  })
  return [...counts.entries()].map(([name, value]) => ({ name, value })).sort((a, b) => b.value - a.value || a.name.localeCompare(b.name, 'de'))
}

function percent(value: number, total: number): string {
  if (total === 0) return '0%'
  return `${Math.round((value / total) * 100)}%`
}

function StatCard({ label, value, hint }: { label: string; value: string | number; hint: string }) {
  return <article className="stat-card">
    <span>{label}</span>
    <strong>{value}</strong>
    <small>{hint}</small>
  </article>
}

export function StatisticsPage() {
  const [raids, setRaids] = useState<Raid[]>([])
  const [players, setPlayers] = useState<Player[]>([])
  const [characters, setCharacters] = useState<Character[]>([])
  const [details, setDetails] = useState<RaidDetails>({})
  const [loading, setLoading] = useState(true)
  const [detailsLoading, setDetailsLoading] = useState(false)
  const [error, setError] = useState('')
  const [detailsError, setDetailsError] = useState('')

  useEffect(() => {
    let active = true
    Promise.all([
      api.get<Raid[]>('/api/v1/raids?include_archived=true'),
      api.get<Player[]>('/api/v1/players'),
      api.get<Character[]>('/api/v1/characters'),
    ])
      .then(([raidRows, playerRows, characterRows]) => {
        if (!active) return
        setRaids(raidRows)
        setPlayers(playerRows)
        setCharacters(characterRows)
      })
      .catch((err) => { if (active) setError(err instanceof ApiError ? err.message : 'Statistiken konnten nicht geladen werden.') })
      .finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [])

  const historicalRaids = raids
    .filter((raid) => raid.archived_at || raid.status === 'COMPLETED')
    .sort((a, b) => Date.parse(b.archived_at ?? b.completed_at ?? b.scheduled_start ?? b.created_at) - Date.parse(a.archived_at ?? a.completed_at ?? a.scheduled_start ?? a.created_at))

  useEffect(() => {
    let active = true
    const raidRows = raids.filter((raid) => raid.archived_at || raid.status === 'COMPLETED')
    if (raidRows.length === 0) {
      setDetails({})
      return () => { active = false }
    }
    setDetailsLoading(true)
    setDetailsError('')
    Promise.all(raidRows.map(async (raid) => {
      const [attendance, loot] = await Promise.all([
        api.get<Attendance[]>(`/api/v1/raids/${raid.id}/attendance`),
        api.get<LootAward[]>(`/api/v1/raids/${raid.id}/loot`),
      ])
      return [raid.id, { attendance, loot }] as const
    }))
      .then((rows) => { if (active) setDetails(Object.fromEntries(rows)) })
      .catch((err) => { if (active) setDetailsError(err instanceof ApiError ? err.message : 'Historische Raid-Details konnten nicht geladen werden.') })
      .finally(() => { if (active) setDetailsLoading(false) })
    return () => { active = false }
  }, [raids])

  const activePlayers = players.filter((player) => player.active).length
  const activeCharacters = characters.filter((character) => character.active)
  const allAttendance = Object.values(details).flatMap((detail) => detail.attendance)
  const allLoot = Object.values(details).flatMap((detail) => detail.loot)
  const mainSpecLoot = allLoot.filter((award) => award.is_main_spec).length
  const benchRows = allAttendance.filter((row) => row.is_bench).length
  const raidStatusData = [
    { name: 'Geplant', value: raids.filter((raid) => raid.status === 'PLANNED' && !raid.archived_at).length },
    { name: 'Offen', value: raids.filter((raid) => raid.status === 'OPEN' && !raid.archived_at).length },
    { name: 'Archiviert', value: raids.filter((raid) => raid.archived_at || raid.status === 'COMPLETED').length },
    { name: 'Abgesagt', value: raids.filter((raid) => raid.status === 'CANCELLED').length },
  ].filter((entry) => entry.value > 0)
  const classData = countBy(activeCharacters, (character) => CLASS_LABELS[character.class] ?? character.class)
  const roleData = countBy(allAttendance, (row) => row.role === 'TANK' ? 'Tank' : row.role === 'HEAL' ? 'Heiler' : 'DD')
  const lootMethodData = countBy(allLoot, (award) => METHOD_LABELS[award.award_method ?? ''] ?? award.award_method ?? 'Unbekannt')
  const playerNames = new Map(players.map((player) => [player.id, player.name]))
  const attendanceLeaders = countBy(allAttendance, (row) => playerNames.get(row.player_id) ?? `Spieler #${row.player_id}`).slice(0, 10)
  const lootLeaders = countBy(allLoot, (award) => playerNames.get(award.awarded_to_player_id) ?? `Spieler #${award.awarded_to_player_id}`).slice(0, 10)

  return <div className="statistics-page">
    <div className="page-heading"><div><p className="eyebrow">Gildenverlauf</p><h1>Statistiken</h1></div><Link to="/raids">Raid-Historie ansehen &rarr;</Link></div>
    {error && <ErrorMessage>{error}</ErrorMessage>}
    {detailsError && <ErrorMessage>{detailsError}</ErrorMessage>}
    {(loading || detailsLoading) && <p className="muted" role="status">Statistiken werden aus gespeicherten Daten berechnet&hellip;</p>}

    <section className="statistics-summary" aria-label="Kennzahlen">
      <StatCard label="Aktive Spieler" value={activePlayers} hint="Spieler mit aktivem Status" />
      <StatCard label="Aktive Charaktere" value={activeCharacters.length} hint="Alle gespeicherten aktiven Charaktere" />
      <StatCard label="Archivierte Raids" value={historicalRaids.length} hint="Abgeschlossene Raid-Historie" />
      <StatCard label="Loot-Einträge" value={allLoot.length} hint={`${mainSpecLoot} Main-Spec / ${allLoot.length - mainSpecLoot} andere`} />
    </section>

    <div className="statistics-grid">
      <section className="card statistics-chart-card">
        <div className="section-heading"><h2>Raids nach Status</h2><span className="eyebrow">Alle gespeicherten Raids</span></div>
        {raidStatusData.length > 0 ? <ResponsiveContainer width="100%" height={260}>
          <PieChart>
            <Pie data={raidStatusData} dataKey="value" nameKey="name" cx="50%" cy="50%" innerRadius={62} outerRadius={92} paddingAngle={3}>
              {raidStatusData.map((_, index) => <Cell key={index} fill={COLORS[index % COLORS.length]} />)}
            </Pie>
            <Tooltip contentStyle={{ background: '#101d2e', border: '1px solid #2a4057', borderRadius: 4 }} labelStyle={{ color: '#dce6f0' }} itemStyle={{ color: '#86d9f2' }} />
            <Legend verticalAlign="bottom" iconType="circle" formatter={(value, entry) => <span style={{ color: '#dce6f0' }}>{value}: {(entry.payload as { value?: number } | undefined)?.value ?? 0}</span>} />
          </PieChart>
        </ResponsiveContainer> : <p className="muted">Noch keine Raids gespeichert.</p>}
      </section>

      <section className="card statistics-chart-card">
        <div className="section-heading"><h2>Klassenverteilung</h2><span className="eyebrow">Aktive Charaktere</span></div>
        {classData.length > 0 ? <ResponsiveContainer width="100%" height={260}>
          <BarChart data={classData} layout="vertical" margin={{ left: 24 }}>
            <XAxis type="number" allowDecimals={false} tick={{ fill: '#9eafc3', fontSize: 11 }} axisLine={{ stroke: '#2a4057' }} tickLine={false} />
            <YAxis type="category" dataKey="name" interval={0} tick={{ fill: '#9eafc3', fontSize: 11 }} axisLine={{ stroke: '#2a4057' }} tickLine={false} />
            <Tooltip contentStyle={{ background: '#101d2e', border: '1px solid #2a4057', borderRadius: 4 }} labelStyle={{ color: '#dce6f0' }} itemStyle={{ color: '#86d9f2' }} />
            <Bar dataKey="value" fill="#86d9f2" radius={[0, 3, 3, 0]} />
          </BarChart>
        </ResponsiveContainer> : <p className="muted">Noch keine aktiven Charaktere gespeichert.</p>}
      </section>

      <section className="card statistics-chart-card">
        <div className="section-heading"><h2>Historische Rollen</h2><span className="eyebrow">Archivierte Roster</span></div>
        {roleData.length > 0 ? <div className="stat-bars">{roleData.map((entry) => <div className="stat-bar-row" key={entry.name}><span>{entry.name}</span><strong>{entry.value}</strong><div><i style={{ width: percent(entry.value, allAttendance.length) }} /></div></div>)}</div> : <p className="muted">Noch keine historischen Aufstellungen vorhanden.</p>}
        {allAttendance.length > 0 && <p className="muted">{benchRows} von {allAttendance.length} Einträgen waren Ersatzbank.</p>}
      </section>

      <section className="card statistics-chart-card">
        <div className="section-heading"><h2>Loot-Verteilung</h2><span className="eyebrow">Vergabemethode</span></div>
        {lootMethodData.length > 0 ? <div className="stat-bars">{lootMethodData.map((entry) => <div className="stat-bar-row" key={entry.name}><span>{entry.name}</span><strong>{entry.value}</strong><div><i style={{ width: percent(entry.value, allLoot.length) }} /></div></div>)}</div> : <p className="muted">Noch keine Loot-Historie vorhanden.</p>}
      </section>

      <section className="card statistics-table-card">
        <div className="section-heading"><h2>Meiste Teilnahmen</h2><span className="eyebrow">Archivierte Raids</span></div>
        <div className="table-scroll"><table className="table small"><thead><tr><th>Spieler</th><th>Einträge</th></tr></thead><tbody>{attendanceLeaders.map((entry) => <tr key={entry.name}><td>{entry.name}</td><td>{entry.value}</td></tr>)}{attendanceLeaders.length === 0 && <tr><td colSpan={2} className="muted">Noch keine Teilnahmen.</td></tr>}</tbody></table></div>
      </section>

      <section className="card statistics-table-card">
        <div className="section-heading"><h2>Meiste Loot-Einträge</h2><span className="eyebrow">Archivierte Raids</span></div>
        <div className="table-scroll"><table className="table small"><thead><tr><th>Spieler</th><th>Loot</th></tr></thead><tbody>{lootLeaders.map((entry) => <tr key={entry.name}><td>{entry.name}</td><td>{entry.value}</td></tr>)}{lootLeaders.length === 0 && <tr><td colSpan={2} className="muted">Noch keine Loot-Einträge.</td></tr>}</tbody></table></div>
      </section>
    </div>
  </div>
}
