import { useEffect, useState } from 'react'
import { api, ApiError } from '../api'
import { useSubmission } from '../useSubmission'
import type { CatalogBoss, LootCatalog } from '../types'

interface Props {
  catalog: LootCatalog
  onSaved: () => void
}

// Admins set the order of an instance's bosses (ADR 088). It applies to the
// boss picker when loot is recorded and to the boss groups of a raid's loot.
export function BossOrderEditor({ catalog, onSaved }: Props) {
  const { pending, submit } = useSubmission()
  const [instanceID, setInstanceID] = useState(catalog.instances[0]?.id ?? '')
  const [order, setOrder] = useState<CatalogBoss[]>([])
  const [notice, setNotice] = useState('')

  // The catalog reloads after a save: keep the "saved" notice then.
  useEffect(() => {
    setOrder(catalog.bosses.filter((boss) => boss.instance_id === instanceID))
  }, [catalog, instanceID])

  const move = (index: number, by: number) => {
    const next = [...order]
    const [boss] = next.splice(index, 1)
    next.splice(index + by, 0, boss)
    setOrder(next)
    setNotice('')
  }

  const save = () => submit(async () => {
    try {
      await api.put<CatalogBoss[]>('/api/v1/loot-catalog/boss-order', { instance_id: instanceID, boss_ids: order.map((boss) => boss.id) })
      setNotice('Reihenfolge gespeichert.')
      onSaved()
    } catch (err) {
      setNotice(err instanceof ApiError ? err.message : 'Reihenfolge konnte nicht gespeichert werden.')
    }
  })

  return <section className="card boss-order-editor"><h2>Boss-Reihenfolge</h2>
    <p className="muted">Gilt für die Boss-Auswahl beim Beute-Eintragen und für die Boss-Gruppen in der Beuteliste eines Raids.</p>
    <label>Instanz<select value={instanceID} onChange={(event) => { setInstanceID(event.target.value); setNotice('') }}>
      {catalog.instances.map((instance) => <option key={instance.id} value={instance.id}>{instance.name}</option>)}
    </select></label>
    <ol className="boss-order-list">{order.map((boss, index) => <li key={boss.id}>
      <span>{boss.name}</span>
      <span className="actions">
        <button type="button" aria-label={`${boss.name} nach oben`} disabled={pending || index === 0} onClick={() => move(index, -1)}>↑</button>
        <button type="button" aria-label={`${boss.name} nach unten`} disabled={pending || index === order.length - 1} onClick={() => move(index, 1)}>↓</button>
      </span>
    </li>)}</ol>
    <div className="actions">
      <button type="button" disabled={pending || order.length === 0} onClick={() => void save()}>Reihenfolge speichern</button>
      {notice && <span role="status">{notice}</span>}
    </div>
  </section>
}
