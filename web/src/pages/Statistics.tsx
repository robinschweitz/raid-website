import { useEffect, useState } from 'react'
import { Bar, BarChart, Cell, Legend, Pie, PieChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { api, ApiError } from '../api'
import { Link } from '../router'
import type { PlayerCount, Statistics } from '../types'
import { ErrorMessage } from '../components/ErrorMessage'

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

const STATUS_LABELS: Record<string, string> = {
  PLANNED: 'Geplant',
  OPEN: 'Offen',
  COMPLETED: 'Archiviert',
  CANCELLED: 'Abgesagt',
}

const ROLE_LABELS: Record<string, string> = { TANK: 'Tank', HEAL: 'Heiler', DD: 'DD' }

function sum(rows: { count: number }[]): number {
  return rows.reduce((total, row) => total + row.count, 0)
}

function StatCard({ label, value, hint }: { label: string; value: string | number; hint: string }) {
  return <article className="stat-card">
    <span>{label}</span>
    <strong>{value}</strong>
    <small>{hint}</small>
  </article>
}

const TOOLTIP_PROPS = {
  contentStyle: { background: '#101d2e', border: '1px solid #2a4057', borderRadius: 4 },
  labelStyle: { color: '#dce6f0' },
  itemStyle: { color: '#86d9f2' },
}

type NamedCount = { name: string; value: number }

// DonutChart and CountBarChart share one height so that cards in the same grid row line up.
function DonutChart({ rows, empty }: { rows: NamedCount[]; empty: string }) {
  if (rows.length === 0) return <p className="muted">{empty}</p>
  return <ResponsiveContainer width="100%" height={260}>
    <PieChart>
      <Pie data={rows} dataKey="value" nameKey="name" cx="50%" cy="50%" innerRadius={62} outerRadius={92} paddingAngle={3}>
        {rows.map((_, index) => <Cell key={index} fill={COLORS[index % COLORS.length]} />)}
      </Pie>
      <Tooltip {...TOOLTIP_PROPS} />
      <Legend verticalAlign="bottom" iconType="circle" formatter={(value, entry) => <span style={{ color: '#dce6f0' }}>{value}: {(entry.payload as { value?: number } | undefined)?.value ?? 0}</span>} />
    </PieChart>
  </ResponsiveContainer>
}

function CountBarChart({ rows, empty }: { rows: NamedCount[]; empty: string }) {
  if (rows.length === 0) return <p className="muted">{empty}</p>
  return <ResponsiveContainer width="100%" height={260}>
    <BarChart data={rows} layout="vertical" margin={{ left: 24 }}>
      <XAxis type="number" allowDecimals={false} tick={{ fill: '#9eafc3', fontSize: 12 }} axisLine={{ stroke: '#2a4057' }} tickLine={false} />
      <YAxis type="category" dataKey="name" interval={0} tick={{ fill: '#9eafc3', fontSize: 12 }} axisLine={{ stroke: '#2a4057' }} tickLine={false} />
      <Tooltip {...TOOLTIP_PROPS} />
      <Bar dataKey="value" fill="#86d9f2" radius={[0, 3, 3, 0]} />
    </BarChart>
  </ResponsiveContainer>
}

function LeaderTable({ rows, label, empty }: { rows: PlayerCount[]; label: string; empty: string }) {
  return <div className="table-scroll"><table className="table small"><thead><tr><th>Spieler</th><th>{label}</th></tr></thead><tbody>{rows.map((entry) => <tr key={entry.player_id}><td>{entry.player_name}</td><td>{entry.count}</td></tr>)}{rows.length === 0 && <tr><td colSpan={2} className="muted">{empty}</td></tr>}</tbody></table></div>
}

export function StatisticsPage() {
  const [stats, setStats] = useState<Statistics | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    let active = true
    api.get<Statistics>('/api/v1/statistics')
      .then((data) => { if (active) setStats(data) })
      .catch((err) => { if (active) setError(err instanceof ApiError ? err.message : 'Statistiken konnten nicht geladen werden.') })
      .finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [])

  const raidStatusData = (stats?.raids_by_status ?? []).map((row) => ({ name: STATUS_LABELS[row.status] ?? row.status, value: row.count })).filter((entry) => entry.value > 0)
  const classData = (stats?.class_distribution ?? []).map((row) => ({ name: CLASS_LABELS[row.class] ?? row.class, value: row.count }))
  const roleData = (stats?.role_history ?? []).map((row) => ({ name: ROLE_LABELS[row.role] ?? row.role, value: row.count }))
  const lootClassData = (stats?.loot_by_class ?? []).map((row) => ({ name: CLASS_LABELS[row.class] ?? row.class, value: row.count }))
  const totalRaids = sum(stats?.raids_by_status ?? [])
  const archivedRaids = stats?.raids_by_status.find((row) => row.status === 'COMPLETED')?.count ?? 0
  const totalCharacters = sum(stats?.class_distribution ?? [])
  const totalLoot = sum(stats?.loot_by_class ?? [])

  return <div className="statistics-page">
    <div className="page-heading"><div><p className="eyebrow">Gildenverlauf</p><h1>Statistiken</h1></div><Link to="/raids">Raid-Historie ansehen &rarr;</Link></div>
    {error && <ErrorMessage>{error}</ErrorMessage>}
    {loading && <p className="muted" role="status">Statistiken werden geladen&hellip;</p>}

    {stats && <>
      <section className="statistics-summary" aria-label="Kennzahlen">
        <StatCard label="Raids" value={totalRaids} hint="Alle gespeicherten Raids" />
        <StatCard label="Archivierte Raids" value={archivedRaids} hint="Abgeschlossene Raid-Historie" />
        <StatCard label="Aktive Charaktere" value={totalCharacters} hint="Alle gespeicherten aktiven Charaktere" />
        <StatCard label="Loot-Einträge" value={totalLoot} hint="Aus archivierten Raids" />
      </section>

      <div className="statistics-grid">
        <section className="card statistics-chart-card">
          <div className="section-heading"><h2>Raids nach Status</h2><span className="eyebrow">Alle gespeicherten Raids</span></div>
          <DonutChart rows={raidStatusData} empty="Noch keine Raids gespeichert." />
        </section>

        <section className="card statistics-chart-card">
          <div className="section-heading"><h2>Klassenverteilung</h2><span className="eyebrow">Aktive Charaktere</span></div>
          <CountBarChart rows={classData} empty="Noch keine aktiven Charaktere gespeichert." />
        </section>

        <section className="card statistics-chart-card">
          <div className="section-heading"><h2>Historische Rollen</h2><span className="eyebrow">Archivierte Roster</span></div>
          <DonutChart rows={roleData} empty="Noch keine historischen Aufstellungen vorhanden." />
        </section>

        <section className="card statistics-chart-card">
          <div className="section-heading"><h2>Loot-Verteilung</h2><span className="eyebrow">Nach Klasse</span></div>
          <CountBarChart rows={lootClassData} empty="Noch keine Loot-Historie vorhanden." />
        </section>

        <section className="card statistics-table-card">
          <div className="section-heading"><h2>Meiste Teilnahmen</h2><span className="eyebrow">Archivierte Raids</span></div>
          <LeaderTable rows={stats.top_attendance} label="Einträge" empty="Noch keine Teilnahmen." />
        </section>

        <section className="card statistics-table-card">
          <div className="section-heading"><h2>Meiste Loot-Einträge</h2><span className="eyebrow">Archivierte Raids</span></div>
          <LeaderTable rows={stats.top_loot} label="Loot" empty="Noch keine Loot-Einträge." />
        </section>
      </div>
    </>}
  </div>
}
