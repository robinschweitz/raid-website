const storageKey = 'raid-support-view'
export interface SupportSession { id: string; target: number; name: string; writable: boolean }
export function supportSession(): SupportSession | null {
  if (typeof sessionStorage === 'undefined') return null
  const value = sessionStorage.getItem(storageKey)
  return value ? JSON.parse(value) as SupportSession : null
}
export function storeSupport(view: SupportSession | null) {
  if (view) sessionStorage.setItem(storageKey, JSON.stringify(view))
  else sessionStorage.removeItem(storageKey)
}
