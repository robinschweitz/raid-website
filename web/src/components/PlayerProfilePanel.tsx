import type { Character, Player, PointsResponse } from '../types'
import { Link } from '../router'

interface PlayerProfilePanelProps {
  player: Player
  characters: Character[]
  points: PointsResponse | null
}

export function PlayerProfilePanel({ player, characters, points }: PlayerProfilePanelProps) {
  const mainChar = characters.find((c) => c.is_main) ?? characters[0]
  const recentTransactions = points?.transactions.slice(-5).reverse() ?? []

  return <div className="profile-panel">
    <div className="profile-avatar" style={{ '--class-color': `var(--class-${mainChar?.class?.toLowerCase() ?? 'none'})` } as React.CSSProperties}>
      {mainChar?.name?.charAt(0) ?? player.name.charAt(0)}
    </div>
    <h3>{player.name}</h3>
    <p className="profile-class character-name">
      {mainChar ? `${mainChar.name} - ${mainChar.class}` : 'Kein Charakter'}
    </p>
    <div className="profile-stats">
      <div className="profile-stat">
        <strong>{points?.balance ?? 0}</strong>
        <span>EP</span>
      </div>
      <div className="profile-stat">
        <strong>{characters.length}</strong>
        <span>Chars</span>
      </div>
    </div>
    <div className="profile-recent">
      <h4>Verlauf</h4>
      {recentTransactions.map((t) => <div className="recent-ep" key={t.id}>
        <span>{t.reason || t.type}</span>
        <span className={`ep-amount ${t.amount > 0 ? 'positive' : t.amount < 0 ? 'negative' : ''}`}>
          {t.amount > 0 ? `+${t.amount}` : t.amount}
        </span>
      </div>)}
      {recentTransactions.length === 0 && <p className="muted">Noch keine Einträge in der Historie.</p>}
    </div>
    <p style={{ marginTop: 12, fontSize: 12 }}>
      <Link to={`/players/${player.id}`}>Vollst&auml;ndiges Profil &rarr;</Link>
    </p>
  </div>
}
