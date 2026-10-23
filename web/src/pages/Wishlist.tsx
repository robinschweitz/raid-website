import { useEffect, useState } from 'react'
import { api, ApiError } from '../api'
import { useAuth } from '../auth'
import { Link } from '../router'
import { CharacterWishlist } from '../components/CharacterWishlist'
import { characterClassName } from '../characterTheme'
import type { Character } from '../types'
import { ErrorMessage } from '../components/ErrorMessage'

// WishlistPage is the member's own wishlist with the addon export, reachable
// from the navigation instead of only via the character page (ADR 081).
export function WishlistPage() {
  const { player } = useAuth()
  const [characters, setCharacters] = useState<Character[] | null>(null)
  const [selectedID, setSelectedID] = useState<number | null>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!player) return
    let active = true
    api.get<Character[]>(`/api/v1/players/${player.id}/characters`)
      .then((rows) => {
        if (!active) return
        const own = rows.filter((row) => row.active)
        setCharacters(own)
        setSelectedID((own.find((row) => row.is_main) ?? own[0])?.id ?? null)
      })
      .catch((err) => { if (active) setError(err instanceof ApiError ? err.message : 'Charaktere konnten nicht geladen werden.') })
    return () => { active = false }
  }, [player])

  const selected = characters?.find((row) => row.id === selectedID)

  return <div>
    <div className="page-heading"><div><p className="eyebrow">Beute & Addon</p><h1>Meine Wunschliste</h1></div><Link to="/loot-catalog">Items im Beute-Katalog hinzufügen &rarr;</Link></div>
    <p className="muted">Items fügst du im <Link to="/loot-catalog">Beute-Katalog</Link> hinzu (Klick auf deinen Charakter beim Item). Hier legst du Spec und Enhanced fest und kopierst die Liste fürs Addon.</p>
    {error && <ErrorMessage>{error}</ErrorMessage>}
    {characters === null && !error && <p className="muted">Lade Charaktere…</p>}
    {characters && characters.length === 0 && <p className="muted">Deinem Account sind noch keine Charaktere zugeordnet. Bitte wende dich an die Gildenleitung.</p>}
    {characters && characters.length > 1 && <div className="wishlist-character-picker" role="group" aria-label="Charakter wählen">
      {characters.map((character) => <button type="button" key={character.id} aria-pressed={character.id === selectedID} onClick={() => setSelectedID(character.id)}>
        <span className={`character-name ${characterClassName(character.class)}`}>{character.name}</span>{character.is_main && <span className="muted"> · Main</span>}
      </button>)}
    </div>}
    {selected && <>
      <h2><span className={`character-name ${characterClassName(selected.class)}`}>{selected.name}</span> <span className="muted">· {selected.class}</span></h2>
      <CharacterWishlist key={selected.id} characterID={selected.id} canEdit />
    </>}
  </div>
}
