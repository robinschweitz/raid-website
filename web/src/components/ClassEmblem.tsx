import { characterClassName } from '../characterTheme'

const iconNames: Record<string, string> = {
  'class-deathknight': 'classicon_deathknight',
  'class-druid': 'classicon_druid',
  'class-hunter': 'classicon_hunter',
  'class-mage': 'classicon_mage',
  'class-paladin': 'classicon_paladin',
  'class-priest': 'classicon_priest',
  'class-shaman': 'classicon_shaman',
  'class-rogue': 'classicon_rogue',
  'class-warlock': 'classicon_warlock',
  'class-warrior': 'classicon_warrior',
}

export function ClassEmblem({ characterClass }: { characterClass: string }) {
  const className = characterClassName(characterClass.trim())
  const iconName = iconNames[className]
  if (!iconName) {
    return <span className="class-emblem class-emblem--unknown" aria-hidden="true" />
  }
  return <img className="class-emblem" src={`https://wow.zamimg.com/images/wow/icons/large/${iconName}.jpg`} alt="" aria-hidden="true" />
}
