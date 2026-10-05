import { characterClassName } from '../characterTheme'

// Original class motifs: runeblade, antlers, bow, crystal, hammer, sun,
// lightning, daggers, demonic eye, and crossed axes.
const motifs: Record<string, string> = {
  'class-deathknight': 'M24 7 29 14 27 31 24 36 21 31 19 14Z M24 15v15m-3-9 6 4m-6 1 6-6M15 32l9 4 9-4M24 36v6m-4 0h8M10 12l4 5-2 8m26-13-4 5 2 8',
  'class-druid': 'M18 24 11 18 9 8m3 11-6-2m6 2 5-7m13 12 7-6 2-10m-3 11 6-2m-6 2-5-7M16 25l8-6 8 6-3 12-5 5-5-5Z M19 28l3 2m7-2-3 2m-4 5h4',
  'class-hunter': 'M14 7c24 7 24 27 0 34l6-17Z M8 24h32m-6-6 6 6-6 6M8 20l5 4-5 4',
  'class-mage': 'M24 5 34 20 24 35 14 20Z M24 5v30M14 20h20M12 34l12 8 12-8M7 11v6m-3-3h6m28 14v6m-3-3h6',
  'class-paladin': 'M10 12h24v11H10Z M17 8h10v4M20 23h7v18h-7ZM7 29l5 3m26-3-5 3M24 2v3m14 3-3 3M9 8l3 3',
  'class-priest': 'M24 7v34M17 19h14M20 37h8M18 25a11 11 0 1 1 12 0M24 2v3M8 8l4 4m28-4-4 4M4 20h5m30 0h5M8 32l4-4m28 4-4-4',
  'class-shaman': 'M27 5 12 27h11l-2 16 15-24H25Z M13 10l-6 6v16l7 6m21-28 6 6v16l-7 6',
  'class-rogue': 'M8 6l13 9 1 13-7-5Z M40 6l-13 9-1 13 7-5ZM12 27l12 9m-7-5-6 9m25-13-12 9m7-5 6 9M9 39l4 3m22 0 4-3',
  'class-warlock': 'M10 19 7 6l12 9m19 4 3-13-12 9M7 26q17-22 34 0-17 22-34 0Z M24 19l4 7-4 7-4-7ZM14 36l3 5m17-5-3 5',
  'class-warrior': 'M10 8l30 32M38 8 8 40M13 11l-6 5 1 10 12-8ZM35 11l6 5-1 10-12-8ZM7 37l4 4m26-4 4 4',
}

export function ClassEmblem({ characterClass }: { characterClass: string }) {
  const motif = motifs[characterClassName(characterClass.trim())]
  return <svg className="class-emblem" viewBox="0 0 48 48" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" focusable="false">
    {motif ? <path d={motif} /> : <><path d="m24 7 16 17-16 17L8 24Z" /><path d="M20 19a4 4 0 0 1 8 0c0 3-4 3-4 7m0 5v1" /></>}
  </svg>
}
