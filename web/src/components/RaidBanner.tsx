const INSTANCE_THEMES: Record<string, { className: string; label: string }> = {
  '533': { className: 'banner-naxxramas', label: 'Naxxramas' },
  '603': { className: 'banner-ulduar', label: 'Ulduar' },
  '624': { className: 'banner-archavon', label: 'Vault of Archavon' },
  '631': { className: 'banner-icecrown', label: 'Icecrown Citadel' },
  '649': { className: 'banner-crusader', label: 'Trial of the Crusader' },
  '724': { className: 'banner-ruby', label: 'Ruby Sanctum' },
}

export function RaidBanner({ catalogInstanceId, children }: { catalogInstanceId?: string | null; children: React.ReactNode }) {
  const theme = catalogInstanceId ? INSTANCE_THEMES[catalogInstanceId] : undefined
  return (
    <section className={`raid-banner ${theme?.className ?? 'banner-default'}`}>
      <div className="raid-banner-bg" aria-hidden="true" />
      <div className="raid-banner-content">{children}</div>
    </section>
  )
}
