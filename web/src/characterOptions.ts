import { useEffect, useState } from 'react'
import { api } from './api'

export function canonicalClass(value: string): string {
  const names: Record<string, string> = {
    'Death Knight': 'Todesritter', Druid: 'Druide', Hunter: 'Jäger', Mage: 'Magier', Priest: 'Priester',
    Shaman: 'Schamane', Rogue: 'Schurke', Warlock: 'Hexer', Warrior: 'Krieger',
  }
  return names[value] ?? value
}

export function useCharacterOptions() {
  const [options, setOptions] = useState<Record<string, string[]>>({})
  const [error, setError] = useState('')
  const [retry, setRetry] = useState(0)
  useEffect(() => {
    let active = true
    setError('')
    api.get<Record<string, string[]>>('/api/v1/character-options').then((rows) => {
      if (active) setOptions(rows)
    }).catch(() => {
      if (active) setError('Klassen und Specs konnten nicht geladen werden.')
    })
    return () => { active = false }
  }, [retry])
  return { options, error, reload: () => setRetry((value) => value + 1) }
}
