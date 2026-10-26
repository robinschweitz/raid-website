import { getBossesForInstance } from './bossData'
import { catalogModeLabels, type BossKill } from '../types'

// kills is null while unknown (not loaded, or the viewer may not read them); the
// component then shows no kill status instead of guessing (FR-29.4, FR-11.5).
export function BossProgression({ catalogInstanceId, kills = null }: { catalogInstanceId?: string | null; kills?: BossKill[] | null }) {
  const bosses = getBossesForInstance(catalogInstanceId)
  if (bosses.length === 0) return <p className="muted">Keine Bossübersicht für diese Instanz verfügbar.</p>

  const killFor = (name: string) => kills?.find((kill) => kill.boss.toLowerCase() === name.toLowerCase())
  const killedCount = bosses.filter((boss) => killFor(boss.name)).length
  const status = kills === null ? 'Killstatus nicht verfügbar'
    : kills.length === 0 ? 'Keine Kills erfasst'
      : `${killedCount} von ${bosses.length} getötet`

  return <div className="boss-progression">
    <span className="boss-label">Bosse</span>
    <ul className="boss-icons" aria-label="Bossbegegnungen der Instanz">
      {bosses.map((boss) => {
        const kill = killFor(boss.name)
        return <li key={boss.name} className={kill ? 'boss-icon killed' : 'boss-icon'} tabIndex={0}>
          <img src={`/bosses/${boss.icon}.webp`} alt={boss.name} width={40} height={40} />
          <span className="boss-tooltip" role="tooltip">{kill ? `${boss.name} · getötet (${catalogModeLabels[kill.difficulty]})` : boss.name}</span>
        </li>
      })}
    </ul>
    <span className="progress-text"><strong>{bosses.length}</strong> Begegnungen · {status}</span>
  </div>
}
