interface BossProgressionProps {
  bosses: string[]
  killedCount: number
  currentIndex?: number
}

export function BossProgression({ bosses, killedCount, currentIndex }: BossProgressionProps) {
  return <div className="boss-progression">
    <span className="boss-label">Progression</span>
    <div className="boss-icons">
      {bosses.map((boss, i) => <div key={boss} className={`boss-icon ${i < killedCount ? 'killed' : ''} ${i === currentIndex ? 'current' : ''}`} title={boss}>
        {i + 1}
      </div>)}
    </div>
    <span className="progress-text"><strong>{killedCount} / {bosses.length}</strong> besiegt</span>
  </div>
}
