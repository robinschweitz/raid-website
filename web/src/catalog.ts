import { type CatalogItemDrop } from './types'

export function matchesCatalogItem(item: CatalogItemDrop, query: string): boolean {
  const text = query.trim().toLocaleLowerCase('de')
  return !text || item.name.toLocaleLowerCase('de').includes(text) || item.item_id.includes(text)
}
