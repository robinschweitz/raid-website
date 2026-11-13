import { useEffect, useState } from 'react'
import { api } from './api'
import type { EpPool } from './types'

/** Loads the public EP pool list (FR-16.9, ADR 113); empty while loading or on error. */
export function useEpPools(reloadKey = 0): EpPool[] {
  const [pools, setPools] = useState<EpPool[]>([])
  useEffect(() => {
    let active = true
    api.get<EpPool[]>('/api/v1/ep-pools')
      .then((data) => { if (active) setPools(data) })
      .catch(() => { if (active) setPools([]) })
    return () => { active = false }
  }, [reloadKey])
  return pools
}
