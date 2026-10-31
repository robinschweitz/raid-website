interface AttendanceRingProps {
  confirmed: number
  requiredPlayers: number
  size?: number
}

export function AttendanceRing({ confirmed, requiredPlayers, size = 100 }: AttendanceRingProps) {
  const percentage = requiredPlayers > 0 ? Math.min(100, Math.round((confirmed / requiredPlayers) * 100)) : 0
  const radius = 40
  const circumference = 2 * Math.PI * radius
  const offset = circumference - (percentage / 100) * circumference

  return <div className="attendance-ring" style={{ width: size, height: size }}>
    <svg width={size} height={size} viewBox="0 0 100 100">
      <circle className="ring-bg" cx="50" cy="50" r={radius} />
      <circle className="ring-fill" cx="50" cy="50" r={radius}
        strokeDasharray={circumference}
        strokeDashoffset={offset} />
    </svg>
    <div className="ring-text">
      <strong>{percentage}%</strong>
      {/* <span>Besucht</span> */}
    </div>
  </div>
}
