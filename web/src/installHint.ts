export type InstallHint = 'none' | 'prompt' | 'ios'

// Which install hint to show (FR-28.5): nothing once installed or hidden, the
// browser's own prompt where it offers one, else manual steps on iPhone/iPad.
export function installHint(state: { isStandalone: boolean; isDismissed: boolean; hasPrompt: boolean; isIOS: boolean }): InstallHint {
  if (state.isStandalone || state.isDismissed) return 'none'
  if (state.hasPrompt) return 'prompt'
  return state.isIOS ? 'ios' : 'none'
}
