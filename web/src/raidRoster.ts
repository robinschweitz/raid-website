import { characterClassName } from './characterTheme'
import type { Attendance } from './types'

export const rosterGroups = [
  { role: 'TANK', label: 'Tanks', symbol: '◇' },
  { role: 'HEAL', label: 'Heilung', symbol: '+' },
  { role: 'MELEE', label: 'Nahkampf', symbol: '⚔' },
  { role: 'RANGED', label: 'Fernkampf', symbol: '◎' },
  { role: 'UNKNOWN', label: 'Schaden · unklar', symbol: '?' },
] as const

type RosterGroup = typeof rosterGroups[number]['role']

export function rosterGroup(row: Pick<Attendance, 'role' | 'class' | 'spec'>): RosterGroup {
  if (row.role === 'TANK' || row.role === 'HEAL') return row.role
  const characterClass = characterClassName(row.class?.trim() ?? '')
  const spec = row.spec?.trim()
  switch (characterClass) {
    case 'class-deathknight':
    case 'class-warrior':
    case 'class-paladin':
    case 'class-rogue':
      return 'MELEE'
    case 'class-hunter':
    case 'class-mage':
    case 'class-priest':
    case 'class-warlock':
      return 'RANGED'
    case 'class-druid':
      if (spec === 'Wilder Kampf (Cat)') return 'MELEE'
      if (spec === 'Gleichgewicht (Eule)') return 'RANGED'
      return 'UNKNOWN'
    case 'class-shaman':
      if (spec === 'Verstärkung') return 'MELEE'
      if (spec === 'Elementar') return 'RANGED'
      return 'UNKNOWN'
    default:
      return 'UNKNOWN'
  }
}
