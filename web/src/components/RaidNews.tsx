import { useEffect, useState, type FormEvent } from 'react'
import { api } from '../api'
import { atLeast, type Player } from '../types'

type News = { id: number; title: string; body: string; important: boolean; published: boolean; acknowledged: boolean; created_at: string }
type Draft = { title: string; body: string; important: boolean; published: boolean }
const emptyDraft: Draft = { title: '', body: '', important: true, published: true }

export function RaidNews({ player }: { player: Player | null }) {
  const [items, setItems] = useState<News[]>([])
  const [manage, setManage] = useState<News[]>([])
  const [editing, setEditing] = useState<number | null>(null)
  const [draft, setDraft] = useState<Draft>(emptyDraft)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const canManage = !!player && atLeast(player.role, 'RAIDASSISTENT')
  const load = async () => {
    const news = await api.get<News[]>('/api/v1/raid-news')
    setItems(news)
    if (canManage) setManage(await api.get<News[]>('/api/v1/raid-news/manage'))
  }
  useEffect(() => {
    let live = true
    api.get<News[]>('/api/v1/raid-news').then((news) => { if (live) setItems(news) }).catch(() => { if (live) setError('Raid-News sind derzeit nicht verfügbar.') })
    if (canManage) api.get<News[]>('/api/v1/raid-news/manage').then((news) => { if (live) setManage(news) }).catch(() => undefined)
    return () => { live = false }
  }, [canManage])
  const pending = player ? items.find((item) => item.important && !item.acknowledged) : undefined
  const acknowledge = async () => {
    if (!pending || busy) return
    setBusy(true)
    try {
      await api.post<void>(`/api/v1/raid-news/${pending.id}/acknowledge`)
      setItems((old) => old.map((item) => item.id === pending.id ? { ...item, acknowledged: true } : item))
    } catch { setError('Bestätigung konnte nicht gespeichert werden. Bitte erneut versuchen.') }
    finally { setBusy(false) }
  }
  const save = async (event: FormEvent) => {
    event.preventDefault()
    if (busy) return
    setBusy(true)
    setError('')
    try {
      if (editing === null) await api.post('/api/v1/raid-news', draft)
      else await api.put(`/api/v1/raid-news/${editing}`, draft)
      await load()
      setEditing(null)
      setDraft(emptyDraft)
    } catch { setError('Raid-News konnten nicht gespeichert werden.') }
    finally { setBusy(false) }
  }
  return <section className="card" aria-label="Raid-News">
    <div className="section-heading"><h2>Raid-News</h2><span className="eyebrow">Mitteilungen</span></div>
    {error && <p role="alert">{error}</p>}
    {items.length === 0 && <p className="muted">Keine Raid-News vorhanden.</p>}
    {items.map((item) => <article key={item.id} style={{ marginBottom: '1rem', overflowWrap: 'anywhere' }}>
      <h3>{item.important ? '📣 ' : ''}{item.title}</h3>
      <p style={{ whiteSpace: 'pre-wrap' }}>{item.body}</p>
      <small className="muted">{new Date(item.created_at).toLocaleDateString('de-DE')}</small>
    </article>)}
    {canManage && <details>
      <summary>Raid-News verwalten</summary>
      <form onSubmit={save} className="inline-form">
        <label>Titel<input maxLength={120} required value={draft.title} onChange={(e) => setDraft((old) => ({ ...old, title: e.target.value }))} /></label>
        <label>Nachricht<textarea maxLength={4000} required rows={4} value={draft.body} onChange={(e) => setDraft((old) => ({ ...old, body: e.target.value }))} /></label>
        <label><input type="checkbox" checked={draft.important} onChange={(e) => setDraft((old) => ({ ...old, important: e.target.checked }))} /> Wichtig (Popup)</label>
        <label><input type="checkbox" checked={draft.published} onChange={(e) => setDraft((old) => ({ ...old, published: e.target.checked }))} /> Veröffentlicht</label>
        <button disabled={busy} type="submit">{editing === null ? 'Veröffentlichen / speichern' : 'Änderungen speichern'}</button>
        {editing !== null && <button type="button" onClick={() => { setEditing(null); setDraft(emptyDraft) }}>Abbrechen</button>}
      </form>
      {manage.map((item) => <div key={item.id} className="actions">
        <span>{item.title}{item.published ? '' : ' (Entwurf)'}</span>
        <button type="button" onClick={() => { setEditing(item.id); setDraft({ title: item.title, body: item.body, important: item.important, published: item.published }) }}>Bearbeiten</button>
      </div>)}
    </details>}
    {pending && <div role="presentation" style={{ position: 'fixed', inset: 0, zIndex: 1000, background: 'rgba(0,0,0,.8)', display: 'grid', placeItems: 'center', padding: 20 }}>
      <div role="alertdialog" aria-modal="true" aria-labelledby="raid-news-dialog-title" aria-describedby="raid-news-dialog-body" className="card" style={{ maxWidth: 520, width: '100%', maxHeight: '90vh', overflowY: 'auto' }}>
        <h2 id="raid-news-dialog-title">{pending.title}</h2>
        <p id="raid-news-dialog-body" style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>{pending.body}</p>
        <button autoFocus type="button" disabled={busy} onClick={acknowledge}>Verstanden</button>
      </div>
    </div>}
  </section>
}
