import { rosterGroup } from './raidRoster'
import { characterClassName } from './characterTheme'
import type { Attendance } from './types'
import { emptyTargets, type RecruitmentRole, type RecruitmentTargets } from './recruitmentText'

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

/** The recruitment role a setup slot belongs to. */
export function slotRole(label: string): RecruitmentRole {
  if (label === 'Maintank' || label === 'Offtank' || label === 'Irgendein Tank') return 'TANK'
  if (label === 'Irgendein Heiler' || /^(Paladin Heilig|Schamane Wiederherstellung|Druide Wiederherstellung|Priester Disziplin)$/.test(label)) return 'HEAL'
  if (label === 'Irgendein MDD' || /^(Krieger|Schurke|Todesritter|Paladin Vergeltung)/.test(label)) return 'MELEE'
  return 'RANGED'
}

/** Short names as they are written in the raid search chat. */
export const slotShortNames: Record<string, string> = {
  Maintank: 'MT', Offtank: 'OT', 'Irgendein Tank': 'beliebig', 'Irgendein Heiler': 'beliebig',
  'Irgendein MDD': 'beliebig', 'Irgendein RDD': 'beliebig',
  'Krieger Furor': 'Fury', 'Schurke Kampf': 'Rogue', 'Todesritter Unheilig': 'Unholy', 'Paladin Vergeltung': 'Ret',
  'Jäger Treffsicherheit': 'Hunter', 'Hexer Dämonologie': 'Hexer', 'Druide Eule': 'Eule', 'Magier Feuer': 'Mage',
  'Schamane Elementar': 'Ele', 'Priester Schatten': 'SP', 'Paladin Heilig': 'Holy Pala',
  'Schamane Wiederherstellung': 'Resto Schami', 'Druide Wiederherstellung': 'Baum', 'Priester Disziplin': 'Diszi',
}

const shortRoleLabels: Record<RecruitmentRole, [string, string]> = {
  TANK: ['Tank', 'Tanks'], HEAL: ['Heal', 'Heal'], MELEE: ['MDD', 'MDD'], RANGED: ['RDD', 'RDD'],
}

/** "Baum, 2x SP": the short names of the missing slots of one role, in setup order. */
export function missingSpecs(missingSlots: { label: string }[], role: RecruitmentRole): string {
  const counts = new Map<string, number>()
  for (const slot of missingSlots) {
    if (slotRole(slot.label) !== role) continue
    const name = slotShortNames[slot.label] ?? slot.label
    counts.set(name, (counts.get(name) ?? 0) + 1)
  }
  // "beliebig" goes last so the specific wishes are read first.
  const names = [...counts.keys()].sort((a, b) => Number(a === 'beliebig') - Number(b === 'beliebig'))
  // The role total already says how many; "2 MDD (beliebig)", not "2 MDD (2x beliebig)".
  if (names.length === 1 && names[0] === 'beliebig') return 'beliebig'
  return names.map((name) => counts.get(name)! > 1 ? `${counts.get(name)}x ${name}` : name).join(', ')
}

/** "2 Heal (Baum, Diszi), 2 MDD (beliebig), 2 RDD (Mage, SP)" */
export function shortSetupSummary(missingSlots: { label: string }[]): string {
  const parts: string[] = []
  for (const role of ['TANK', 'HEAL', 'MELEE', 'RANGED'] as const) {
    const count = missingSlots.filter((slot) => slotRole(slot.label) === role).length
    if (!count) continue
    const [singular, plural] = shortRoleLabels[role]
    parts.push(`${count} ${count === 1 ? singular : plural} (${missingSpecs(missingSlots, role)})`)
  }
  return parts.length ? parts.join(', ') : 'keine offenen Plätze'
}

/** Fills the setup placeholders of a recruitment template. */
export function renderSetupPlaceholders(template: string, missingSlots: { label: string }[]): string {
  const roleTokens: Record<string, RecruitmentRole> = { TANK: 'TANK', HEAL: 'HEAL', MDD: 'MELEE', RDD: 'RANGED' }
  return template
    .replace(/{{SETUP_KURZ}}/g, () => shortSetupSummary(missingSlots))
    .replace(/{{SETUP}}/g, () => missingSlots.length ? missingSlots.map((slot) => slot.label).join(', ') : 'keine offenen Plätze')
    .replace(/{{(TANK|HEAL|MDD|RDD)_SPECS}}/g, (_, token: string) => missingSpecs(missingSlots, roleTokens[token]) || '-')
}

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
      // A flexible requirement must never displace an already fulfilled specific spec.
      if (previous !== undefined && slots[slotIndex].label.startsWith('Irgendein ') && !slots[previous].label.startsWith('Irgendein ')) continue
      if (previous === undefined || assign(previous, visited)) {
        matchedPlayerToSlot.set(playerIndex, slotIndex)
        return true
      }
    }
    return false
  }
  // Fill specific slots first and protect them from flexible-slot reassignment.
  const ordered = slots.map((_, index) => index).sort((left, right) =>
    Number(slots[left].label.startsWith('Irgendein ')) - Number(slots[right].label.startsWith('Irgendein ')) || candidates[left].length - candidates[right].length || slots[left].index - slots[right].index)
  for (const slotIndex of ordered) assign(slotIndex, new Set())
  const matched = new Set(matchedPlayerToSlot.values())
  const missingSlots = slots.filter((_, index) => !matched.has(index))
  const missing: RecruitmentTargets = emptyTargets()
  for (const slot of missingSlots) missing[slotRole(slot.label)]++
  return { missingSlots, missing, matchedIndices: new Set(slots.filter((_, index) => matched.has(index)).map((slot) => slot.index)) }
}
