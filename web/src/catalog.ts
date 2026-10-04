import { type CatalogItemDrop } from './types'

export function matchesCatalogItem(item: CatalogItemDrop, query: string): boolean {
  const text = query.trim().toLocaleLowerCase('de')
  return !text || item.name.toLocaleLowerCase('de').includes(text) || item.item_id.includes(text)
}

// Labels describe source alternatives, not conditions checked by the website.
export function catalogSourceLabels(item: CatalogItemDrop): string[] {
  const labels = new Set<string>()
  for (const variant of item.source_variants) {
    const parts = variant.difficulty.split('/').map((part) => part.trim())
    const chest = parts.find((part) => part.startsWith('source-chest:'))?.split(':')[1]
    const elders = parts.find((part) => part.startsWith('elders:'))?.split(':')[1]
    const attempts = parts.find((part) => part.startsWith('attempts:'))?.split(':')[1]
    const conditions = [elders !== undefined ? `${elders} Älteste bei Kampfbeginn` : '', attempts ? `${attempts} verbleibende Versuche` : ''].filter(Boolean)
    labels.add([chest ? `Truhe #${chest}` : 'Kreaturenbeute', ...conditions].join(' · '))
  }
  return [...labels]
}
