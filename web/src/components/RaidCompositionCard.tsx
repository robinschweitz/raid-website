import type { Attendance } from '../types'

interface RaidCompositionCardProps {
  attendance: Attendance[]
}

export function RaidCompositionCard({ attendance }: RaidCompositionCardProps) {
  const tanks = attendance.filter((a) => a.role === 'TANK').length
  const healers = attendance.filter((a) => a.role === 'HEAL').length
  const dd = attendance.filter((a) => a.role === 'DD').length
  const total = attendance.length

  return <div className="composition-roles">
    <div className="composition-role">
      <div className="role-icon">&#9876;</div>
      <div className="role-count">{tanks}</div>
      <div className="role-label">Tanks</div>
    </div>
    <div className="composition-role">
      <div className="role-icon">&#10010;</div>
      <div className="role-count">{healers}</div>
      <div className="role-label">Heiler</div>
    </div>
    <div className="composition-role">
      <div className="role-icon">&#9876;</div>
      <div className="role-count">{dd}</div>
      <div className="role-label">DD</div>
    </div>
    <div className="composition-role">
      <div className="role-icon">&#9876;</div>
      <div className="role-count">{total}</div>
      <div className="role-label">Gesamt</div>
    </div>
  </div>
}
