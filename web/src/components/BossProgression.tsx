import { getBossesForInstance } from './bossData'

export function BossProgression({ catalogInstanceId }: { catalogInstanceId?: string | null }) {
  const bosses = getBossesForInstance(catalogInstanceId)
  if (bosses.length === 0) return <p className="muted">Keine Bossübersicht für diese Instanz verfügbar.</p>

  return <div className="boss-progression">
    <span className="boss-label">Bosse</span>
    <ul className="boss-icons" aria-label="Bossbegegnungen der Instanz">
      {bosses.map((boss) => <li key={boss.name} className="boss-icon" tabIndex={0}>
        <img src={`/bosses/${boss.icon}.webp`} alt={boss.name} width={40} height={40} />
        <span className="boss-tooltip" role="tooltip">{boss.name}</span>
      </li>)}
    </ul>
    <span className="progress-text"><strong>{bosses.length}</strong> Begegnungen · Killstatus nicht erfasst</span>
  </div>
}
