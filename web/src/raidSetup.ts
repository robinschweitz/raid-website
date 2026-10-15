import { rosterGroup } from './raidRoster'
import { characterClassName } from './characterTheme'
import type { Attendance } from './types'
import { emptyTargets, type RecruitmentTargets } from './recruitmentText'

export const iccIdealSetup = [
  ['Maintank', 'Offtank', 'Krieger Furor', 'Schurke Kampf', 'Todesritter Unheilig'],
  ['Paladin Vergeltung', 'Paladin Vergeltung', 'Irgendein MDD', 'Irgendein MDD', 'Irgendein MDD'],
  ['Jäger Treffsicherheit', 'Hexer Dämonologie', 'Druide Eule', 'Druide Eule', 'Magier Feuer'],
  ['Schamane Elementar', 'Priester Schatten', 'Priester Schatten', 'Irgendein RDD', 'Irgendein RDD'],
  ['Paladin Heilig', 'Schamane Wiederherstellung', 'Druide Wiederherstellung', 'Priester Disziplin', 'Irgendein Heiler'],
]

export const setupOptions = [
  'Leer', 'Maintank', 'Offtank', 'Irgendein Tank', 'Irgendein Heiler', 'Irgendein MDD', 'Irgendein RDD',
  'Krieger Furor', 'Schurke Kampf', 'Todesritter Unheilig', 'Paladin Vergeltung',
  'Jäger Treffsicherheit', 'Hexer Dämonologie', 'Druide Eule', 'Magier Feuer',
  'Schamane Elementar', 'Priester Schatten', 'Paladin Heilig', 'Schamane Wiederherstellung',
  'Druide Wiederherstellung', 'Priester Disziplin',
] as const

export type RaidSetup = string[][]

const classTokens: Record<string, string> = {
  Krieger: 'class-warrior', Schurke: 'class-rogue', Todesritter: 'class-deathknight',
  Paladin: 'class-paladin', Jäger: 'class-hunter', Hexer: 'class-warlock',
  Druide: 'class-druid', Magier: 'class-mage', Schamane: 'class-shaman', Priester: 'class-priest',
}

function normalized(text: string): string {
  return text.trim().toLocaleLowerCase('de-DE').normalize('NFD').replace(/[\u0300-\u036f]/g, '')
}

function isMatching(row: Attendance, slot: string): boolean {
  const group = rosterGroup(row)
  if (slot === 'Maintank' || slot === 'Offtank' || slot === 'Irgendein Tank') return group === 'TANK'
  if (slot === 'Irgendein Heiler') return group === 'HEAL'
  if (slot === 'Irgendein MDD') return group === 'MELEE'
  if (slot === 'Irgendein RDD') return group === 'RANGED'
  const [className, ...specParts] = slot.split(' ')
  if (characterClassName(row.class?.trim() ?? '') !== classTokens[className]) return false
  const spec = normalized(row.spec ?? '')
  const requested = normalized(specParts.join(' '))
  // The existing roster can contain verbose spec descriptions (e.g. Gleichgewicht (Eule)).
  if (requested === 'eule') return spec.includes('eule') || spec.includes('gleichgewicht')
  if (requested === 'kampf') return spec.includes('kampf')
  return spec.includes(requested)
}

export function matchRaidSetup(setup: RaidSetup, attendance: Attendance[]) {
  const slots = setup.flat().map((label, index) => ({ label, index })).filter((slot) => slot.label !== 'Leer')
  const players = attendance.filter((row) => !row.is_bench)
  const candidates = slots.map((slot) => players.map((player, index) => isMatching(player, slot.label) ? index : -1).filter((index) => index >= 0))
  const matchedPlayerToSlot = new Map<number, number>()
  const assign = (slotIndex: number, visited: Set<number>): boolean => {
    for (const playerIndex of candidates[slotIndex]) {
      if (visited.has(playerIndex)) continue
      visited.add(playerIndex)
      const previous = matchedPlayerToSlot.get(playerIndex)
      if (previous === undefined || assign(previous, visited)) {
        matchedPlayerToSlot.set(playerIndex, slotIndex)
        return true
      }
    }
    return false
  }
  // Fill specific slots first; generic slots never consume players needed by specific slots.
  const ordered = slots.map((_, index) => index).sort((left, right) =>
    candidates[left].length - candidates[right].length || slots[left].index - slots[right].index)
  for (const slotIndex of ordered) assign(slotIndex, new Set())
  const matched = new Set(matchedPlayerToSlot.values())
  const missingSlots = slots.filter((_, index) => !matched.has(index))
  const missing: RecruitmentTargets = emptyTargets()
  for (const slot of missingSlots) {
    if (slot.label === 'Maintank' || slot.label === 'Offtank' || slot.label === 'Irgendein Tank') missing.TANK++
    else if (slot.label === 'Irgendein Heiler' || /^(Paladin Heilig|Schamane Wiederherstellung|Druide Wiederherstellung|Priester Disziplin)$/.test(slot.label)) missing.HEAL++
    else if (slot.label === 'Irgendein MDD' || /^(Krieger|Schurke|Todesritter|Paladin Vergeltung)/.test(slot.label)) missing.MELEE++
    else missing.RANGED++
  }
  return { missingSlots, missing, matchedIndices: new Set(slots.filter((_, index) => matched.has(index)).map((slot) => slot.index)) }
}
