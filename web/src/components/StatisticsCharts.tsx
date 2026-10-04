import { BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer, PieChart, Pie, Cell } from 'recharts'
import type { Raid } from '../types'

interface StatisticsChartsProps {
  raids: Raid[]
}

const COLORS = ['#86d9f2', '#8bd4b0', '#ff969c', '#8acdea', '#f5a9d0', '#abd473']

export function StatisticsCharts({ raids }: StatisticsChartsProps) {
  const completedRaids = raids.filter((r) => r.archived_at || r.status === 'COMPLETED')
  const cancelledRaids = raids.filter((r) => r.status === 'CANCELLED')
  const activeRaids = raids.filter((r) => !r.archived_at && (r.status === 'PLANNED' || r.status === 'OPEN'))

  const raidSizeData = [
    { name: 'ICC 25', value: raids.filter((r) => r.raid_size === 25).length },
    { name: 'ICC 10', value: raids.filter((r) => r.raid_size === 10).length },
    { name: 'Andere', value: raids.filter((r) => r.raid_size !== 25 && r.raid_size !== 10).length },
  ].filter((d) => d.value > 0)

  const statusData = [
    { name: 'Aktiv', value: activeRaids.length },
    { name: 'Abgeschlossen', value: completedRaids.length },
    { name: 'Abgesagt', value: cancelledRaids.length },
  ].filter((d) => d.value > 0)

  return <div>
    <div className="chart-container">
      <div className="chart-title">Raids nach Status</div>
      <ResponsiveContainer width="100%" height={180}>
        <PieChart>
          <Pie data={statusData} dataKey="value" nameKey="name" cx="50%" cy="50%"
            innerRadius={45} outerRadius={70} paddingAngle={3}>
            {statusData.map((_, i) => <Cell key={i} fill={COLORS[i % COLORS.length]} />)}
          </Pie>
          <Tooltip contentStyle={{ background: '#101d2e', border: '1px solid #2a4057', borderRadius: 4 }}
            labelStyle={{ color: '#dce6f0' }} itemStyle={{ color: '#86d9f2' }} />
        </PieChart>
      </ResponsiveContainer>
    </div>
    <div className="chart-container">
      <div className="chart-title">Raids nach Gr&ouml;&szlig;e</div>
      <ResponsiveContainer width="100%" height={160}>
        <BarChart data={raidSizeData}>
          <XAxis dataKey="name" tick={{ fill: '#9eafc3', fontSize: 11 }} axisLine={{ stroke: '#2a4057' }} tickLine={false} />
          <YAxis tick={{ fill: '#9eafc3', fontSize: 11 }} axisLine={{ stroke: '#2a4057' }} tickLine={false} allowDecimals={false} />
          <Tooltip contentStyle={{ background: '#101d2e', border: '1px solid #2a4057', borderRadius: 4 }}
            labelStyle={{ color: '#dce6f0' }} itemStyle={{ color: '#86d9f2' }} />
          <Bar dataKey="value" fill="#86d9f2" radius={[3, 3, 0, 0]} />
        </BarChart>
      </ResponsiveContainer>
    </div>
  </div>
}
