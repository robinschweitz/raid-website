import type { ShadowmourneRaidTaskName, ShadowmourneStage } from './types'

// Questline order (ADR 086). Staff may still pick any stage.
export const shadowmourneStages: ShadowmourneStage[] = ['PREQUEST', 'SOULS', 'UNHOLY', 'BLOOD', 'FROST', 'SHARDS', 'FINAL']

export const shadowmourneStageLabels: Record<ShadowmourneStage, string> = {
  PREQUEST: 'Vorquest',
  SOULS: 'Seelen',
  UNHOLY: 'Seuche',
  BLOOD: 'Blut',
  FROST: 'Frost',
  SHARDS: 'Splitter',
  FINAL: 'Abschluss',
}

// Only one raid member can work on these per raid.
export const shadowmourneRaidTasks: ShadowmourneRaidTaskName[] = ['UNHOLY', 'BLOOD', 'FROST', 'SHARDS']

export const shadowmourneTaskHints: Record<ShadowmourneRaidTaskName, string> = {
  UNHOLY: 'Professor Seuchenmord',
  BLOOD: "Blutkönigin Lana'thel",
  FROST: 'Sindragosa',
  SHARDS: 'Schattenfrostsplitter',
}

export const shadowmourneClasses = ['Krieger', 'Paladin', 'Todesritter']

export const defaultShardTarget = 50
