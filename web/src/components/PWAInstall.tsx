import { useEffect, useState } from 'react'
import { installHint } from '../installHint'
import { NavIcon } from './NavIcon'

// The event Chromium browsers fire when the site can be installed.
type InstallPromptEvent = Event & { prompt: () => Promise<void> }

const dismissKey = 'pwa-install-dismissed'

function isHintDismissed(): boolean {
  try {
    return window.sessionStorage?.getItem(dismissKey) === '1'
  } catch {
    return false
  }
}

function dismissHint() {
  try {
    window.sessionStorage?.setItem(dismissKey, '1')
  } catch {
    // Without storage the hint is hidden until the next page load only.
  }
}

function isStandaloneDisplay(): boolean {
  const standaloneQuery = typeof window.matchMedia === 'function' && window.matchMedia('(display-mode: standalone)').matches
  const iosStandalone = typeof navigator !== 'undefined' && (navigator as Navigator & { standalone?: boolean }).standalone === true
  return standaloneQuery || iosStandalone
}

function isIOSDevice(): boolean {
  if (typeof navigator === 'undefined') return false
  // iPadOS reports itself as a Mac but has touch.
  return /iPad|iPhone|iPod/.test(navigator.userAgent) || (navigator.userAgent.includes('Macintosh') && navigator.maxTouchPoints > 1)
}

// Optional "add to home screen" hint (FR-28.5, ADR 101). The service worker it
// registers caches nothing, so private pages never end up stored on the device.
export function PWAInstall() {
  const [prompt, setPrompt] = useState<InstallPromptEvent | null>(null)
  const [isStandalone, setIsStandalone] = useState(false)
  const [isIOS, setIsIOS] = useState(false)
  const [isDismissed, setIsDismissed] = useState(isHintDismissed)

  useEffect(() => {
    if (typeof navigator !== 'undefined' && 'serviceWorker' in navigator && window.isSecureContext) {
      navigator.serviceWorker.register('/sw.js').catch(() => {
        // Without a service worker the website works the same; only installation is unavailable.
      })
    }
    setIsStandalone(isStandaloneDisplay())
    setIsIOS(isIOSDevice())
    if (typeof window.addEventListener !== 'function') return undefined
    const onPrompt = (event: Event) => {
      event.preventDefault()
      setPrompt(event as InstallPromptEvent)
    }
    const onInstalled = () => {
      setIsStandalone(true)
      setPrompt(null)
    }
    window.addEventListener('beforeinstallprompt', onPrompt)
    window.addEventListener('appinstalled', onInstalled)
    return () => {
      window.removeEventListener('beforeinstallprompt', onPrompt)
      window.removeEventListener('appinstalled', onInstalled)
    }
  }, [])

  const hint = installHint({ isStandalone, isDismissed, hasPrompt: prompt !== null, isIOS })
  if (hint === 'none') return null
  const dismiss = () => {
    dismissHint()
    setIsDismissed(true)
  }
  return <div className="install-hint" role="note" aria-label="Hinweis zur Installation">
    <NavIcon name="smartphone" />
    <p className="install-hint-text">
      <strong>RAID vom Startbildschirm öffnen.</strong>
      {hint === 'prompt'
        ? <span> Freiwillig und ohne App Store; die Website funktioniert auch ohne.</span>
        : <span> In Safari auf „Teilen“ tippen und „Zum Home-Bildschirm“ wählen. Freiwillig; die Website funktioniert auch ohne.</span>}
    </p>
    {hint === 'prompt' && prompt && <button type="button" onClick={() => {
      void prompt.prompt().finally(() => setPrompt(null))
    }}>Installieren</button>}
    <button type="button" className="install-hint-close" aria-label="Hinweis für diese Sitzung ausblenden" title="Für diese Sitzung ausblenden" onClick={dismiss}><NavIcon name="x" /></button>
  </div>
}
