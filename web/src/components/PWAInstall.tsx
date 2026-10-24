import { useEffect, useState } from 'react'

type InstallPromptEvent = Event & { prompt: () => Promise<void> }

export function PWAInstall() {
  const [prompt, setPrompt] = useState<InstallPromptEvent | null>(null)
  const [isIOS, setIsIOS] = useState(false)
  const [dismissed, setDismissed] = useState(() => sessionStorage.getItem('pwa-install-dismissed') === '1')
  const [installed, setInstalled] = useState(false)

  useEffect(() => {
    if ('serviceWorker' in navigator && window.isSecureContext) {
      void navigator.serviceWorker.register('/sw.js').catch(() => { /* Website bleibt ohne SW nutzbar. */ })
    }
    const standalone = window.matchMedia('(display-mode: standalone)').matches ||
      ('standalone' in navigator && (navigator as Navigator & { standalone?: boolean }).standalone === true)
    setInstalled(standalone)
    setIsIOS(/iPad|iPhone|iPod/.test(navigator.userAgent))
    const onPrompt = (event: Event) => {
      event.preventDefault()
      setPrompt(event as InstallPromptEvent)
    }
    const onInstalled = () => { setInstalled(true); setPrompt(null) }
    window.addEventListener('beforeinstallprompt', onPrompt)
    window.addEventListener('appinstalled', onInstalled)
    return () => {
      window.removeEventListener('beforeinstallprompt', onPrompt)
      window.removeEventListener('appinstalled', onInstalled)
    }
  }, [])

  if (installed || dismissed || (!prompt && !isIOS)) return null
  const dismiss = () => { sessionStorage.setItem('pwa-install-dismissed', '1'); setDismissed(true) }
  return (
    <section aria-label="Website zum Startbildschirm hinzufügen" style={{ padding: '0.75rem 1rem', margin: '0.75rem', border: '1px solid var(--border, #52647a)', borderRadius: '0.75rem' }}>
      <strong>RAID direkt vom Startbildschirm öffnen</strong>
      <p>Freiwillig und ohne App Store. Die Website funktioniert auch ohne Installation.</p>
      {prompt ? <button type="button" onClick={() => { void prompt.prompt().finally(() => setPrompt(null)) }}>RAID installieren</button> :
        <p>Auf dem iPhone: In Safari auf „Teilen“ tippen und „Zum Home-Bildschirm“ wählen.</p>}
      {' '}<button type="button" onClick={dismiss}>Nicht jetzt</button>
    </section>
  )
}
