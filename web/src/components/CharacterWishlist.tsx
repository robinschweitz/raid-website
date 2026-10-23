import { useEffect, useState } from 'react'
import { api, ApiError } from '../api'
import { useSubmission } from '../useSubmission'
import { Link } from '../router'
import type { WishlistAddonExport, WishlistSpec, WishlistWish } from '../types'
import { ErrorMessage } from './ErrorMessage'

interface Props {
  characterID: number
  canEdit: boolean
}

const specLabels: Record<WishlistSpec, string> = { MAIN: 'Main-Spec', SECOND: 'Second-Spec' }

type RollChange = Partial<Pick<WishlistWish, 'spec' | 'is_enhanced' | 'priority'>>

// CharacterWishlist shows a character's wishlist with the roll the owner wants
// per item and exports it for the RaidWishlist addon (WL2, ADR 070).
export function CharacterWishlist({ characterID, canEdit }: Props) {
  const { pending, submit, isActive } = useSubmission()
  const [wishes, setWishes] = useState<WishlistWish[] | null>(null)
  const [error, setError] = useState('')
  const [exportText, setExportText] = useState('')
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    let active = true
    api.get<WishlistWish[]>(`/api/v1/characters/${characterID}/wishlist`)
      .then((rows) => { if (active) setWishes(rows) })
      .catch((err) => { if (active) setError(err instanceof ApiError ? err.message : 'Wunschliste konnte nicht geladen werden') })
    return () => { active = false }
  }, [characterID])

  const itemPath = (wish: WishlistWish) => `/api/v1/characters/${characterID}/wishlist/items/${encodeURIComponent(wish.item_id)}`

  const save = async (wish: WishlistWish, change: RollChange) => {
    const next = { ...wish, ...change }
    // Enhanced rolls exist only for main spec.
    if (next.spec === 'SECOND') next.is_enhanced = false
    await submit(async () => {
      setError('')
      try {
        const saved = await api.put<WishlistWish>(itemPath(wish), { priority: next.priority, notes: next.notes, spec: next.spec, is_enhanced: next.is_enhanced })
        if (!isActive()) return
        setWishes((rows) => (rows ?? []).map((row) => row.id === saved.id ? saved : row).sort((a, b) => b.priority - a.priority || a.id - b.id))
        setExportText('')
        setCopied(false)
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Wunsch konnte nicht gespeichert werden')
      }
    })
  }

  const remove = async (wish: WishlistWish) => {
    await submit(async () => {
      setError('')
      try {
        await api.del(itemPath(wish))
        if (!isActive()) return
        setWishes((rows) => (rows ?? []).filter((row) => row.id !== wish.id))
        setExportText('')
        setCopied(false)
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Wunsch konnte nicht entfernt werden')
      }
    })
  }

  // One click fetches the current export (with today's EP) and copies it.
  const copyExport = async () => {
    await submit(async () => {
      setError('')
      setCopied(false)
      try {
        const result = await api.get<WishlistAddonExport>(`/api/v1/characters/${characterID}/wishlist/addon-export`)
        if (!isActive()) return
        setExportText(result.text)
        try {
          await navigator.clipboard.writeText(result.text)
          if (isActive()) setCopied(true)
        } catch {
          if (isActive()) setError('Kopieren war nicht möglich. Bitte den Text unten markieren und mit Strg+C kopieren.')
        }
      } catch (err) {
        if (isActive()) setError(err instanceof ApiError ? err.message : 'Export konnte nicht geladen werden')
      }
    })
  }

  const rows = wishes ?? []
  const topPriority = rows.reduce((top, row) => Math.max(top, row.priority), 0)

  return <section className="card character-wishlist" aria-label="Wunschliste">
    <div className="section-heading"><div><h2>Wunschliste</h2><p className="muted">Reihenfolge, Spec und Enhanced gelten für das Wunschlisten-Addon. Enhanced würfelt 1+X bis 100+X mit X = EP-Stand, höchstens 50.</p></div><span className="badge info">{rows.length}</span></div>
    {error && <ErrorMessage>{error}</ErrorMessage>}
    {wishes === null && !error && <p className="muted">Lade Wunschliste…</p>}
    {wishes !== null && rows.length > 0 && <div className="addon-export-box" aria-label="Addon-Export">
      <h3>Ins Wunschlisten-Addon übernehmen</h3>
      <ol>
        <li>Auf <strong>„Export kopieren“</strong> klicken.</li>
        <li>Im Spiel <code>/wish import</code> eingeben.</li>
        <li>Mit <strong>Strg+V</strong> einfügen und bestätigen.</li>
      </ol>
      <div className="actions">
        <button type="button" className="primary-action" disabled={pending} onClick={() => void copyExport()}>Export kopieren</button>
        {copied && <span className="badge ok" role="status">Kopiert – jetzt im Spiel /wish import</span>}
      </div>
      <p className="muted">Der Export enthält deinen aktuellen EP-Stand. Nach neuen EP oder Änderungen einfach erneut kopieren. Noch kein Addon? <Link to="/addons/raidwishlist">RaidWishlist herunterladen</Link></p>
    </div>}
    {wishes !== null && <div className="table-scroll"><table className="table small">
      <thead><tr><th>Item</th><th>Spec</th><th>Enhanced</th>{canEdit && <th>Aktionen</th>}</tr></thead>
      <tbody>{rows.map((wish, index) => <tr key={wish.id}>
        <td><strong className="loot-item-name">{wish.item_name}</strong> <span className="muted">· {wish.item_id}</span>{wish.notes && <div className="muted">{wish.notes}</div>}</td>
        <td>{canEdit
          ? <select aria-label={`Spec für ${wish.item_name}`} disabled={pending} value={wish.spec} onChange={(e) => void save(wish, { spec: e.target.value as WishlistSpec })}>
            {Object.entries(specLabels).map(([spec, label]) => <option key={spec} value={spec}>{label}</option>)}
          </select>
          : specLabels[wish.spec]}</td>
        <td>{canEdit
          ? <label className="check"><input type="checkbox" aria-label={`Enhanced für ${wish.item_name}`} disabled={pending || wish.spec !== 'MAIN'} checked={wish.is_enhanced} onChange={(e) => void save(wish, { is_enhanced: e.target.checked })} /> Enhanced</label>
          : wish.is_enhanced ? 'Ja' : 'Nein'}</td>
        {canEdit && <td className="actions">
          <button type="button" disabled={pending || index === 0} onClick={() => void save(wish, { priority: topPriority + 1 })}>Nach oben</button>
          <button type="button" disabled={pending} onClick={() => void remove(wish)}>Entfernen</button>
        </td>}
      </tr>)}{rows.length === 0 && <tr><td colSpan={canEdit ? 4 : 3} className="muted">{canEdit ? <>Noch keine Wünsche. Im <Link to="/loot-catalog">Beute-Katalog</Link> bei einem Item auf deinen Charakter klicken.</> : 'Keine Wünsche.'}</td></tr>}</tbody>
    </table></div>}
    {exportText && <textarea className="addon-code-export" aria-label="Wunschlisten-Export" readOnly value={exportText} rows={Math.min(8, rows.length + 1)} onFocus={(event) => event.target.select()} />}
  </section>
}
