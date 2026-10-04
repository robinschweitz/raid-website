import { catalogRaidSizes, type LootCatalog } from '../types'

interface Props {
  catalog: LootCatalog | null
  instanceID: string
  raidSize: string
  disabled: boolean
  onChange: (instanceID: string, raidSize: string) => void
  existingInstanceID?: string
  existingRaidSize?: string
}

export function CatalogAssociationFields({ catalog, instanceID, raidSize, disabled, onChange, existingInstanceID = '', existingRaidSize = '' }: Props) {
  const sizes = catalogRaidSizes(catalog, instanceID)
  return <>
    <label>Beutekatalog-Instanz (optional)<select aria-label="Beutekatalog-Instanz" value={instanceID} disabled={disabled || !catalog} onChange={(event) => onChange(event.target.value, '')}>
      <option value="" disabled={!!existingInstanceID}>Keine Zuordnung</option>
      {instanceID && !catalog?.instances.some((instance) => instance.id === instanceID) && <option value={instanceID}>Gespeicherte Instanz #{instanceID} (nicht verfügbar)</option>}
      {catalog?.instances.map((instance) => <option key={instance.id} value={instance.id}>{instance.name}</option>)}
    </select></label>
    <label>Raidgröße<select aria-label="Katalog-Raidgröße" value={raidSize} disabled={disabled || !catalog || !instanceID} onChange={(event) => onChange(instanceID, event.target.value)}>
      <option value="" disabled={!!existingRaidSize}>Größe auswählen</option>
      {raidSize && !sizes.includes(Number(raidSize)) && <option value={raidSize}>{raidSize} Spieler (gespeichert, nicht verfügbar)</option>}
      {sizes.map((size) => <option key={size} value={size}>{size} Spieler</option>)}
    </select></label>
    {catalog && catalog.instances.length === 0 && <p className="muted">Der lokale Beutekatalog ist leer. Ohne Zuordnung bleiben bestehende Raidabläufe nutzbar.</p>}
    {existingInstanceID && <p className="muted">Gespeicherte Zuordnungen bleiben erhalten. Entfernen einer Zuordnung wird hier nicht unterstützt.</p>}
  </>
}
