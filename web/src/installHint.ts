export type InstallHint = 'none' | 'prompt' | 'ios'

// Which install hint to show (FR-28.5): nothing once installed or hidden, the
// browser's own prompt where it offers one, else manual steps on iPhone/iPad.
export function installHint(state: { isStandalone: boolean; isDismissed: boolean; hasPrompt: boolean; isIOS: boolean }): InstallHint {
  if (state.isStandalone || state.isDismissed) return 'none'
  if (state.hasPrompt) return 'prompt'
  return state.isIOS ? 'ios' : 'none'
}

export function isStandaloneDisplay(): boolean {
  const standaloneQuery = typeof window.matchMedia === 'function' && window.matchMedia('(display-mode: standalone)').matches
  const iosStandalone = typeof navigator !== 'undefined' && (navigator as Navigator & { standalone?: boolean }).standalone === true
  return standaloneQuery || iosStandalone
}

export function isIOSDevice(): boolean {
  if (typeof navigator === 'undefined') return false
  // iPadOS reports itself as a Mac but has touch.
  return /iPad|iPhone|iPod/.test(navigator.userAgent) || (navigator.userAgent.includes('Macintosh') && navigator.maxTouchPoints > 1)
}
