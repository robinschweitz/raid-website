import type { EpPool } from '../types'

/** Pool choice for EP views; shows nothing while only one pool exists (FR-16.9). */
export function PoolSelect({ pools, value, onChange, label = 'EP-Topf' }: { pools: EpPool[]; value: number | null; onChange: (poolID: number) => void; label?: string }) {
  if (pools.length < 2) return null
  return <label className="pool-select">{label}
    <select value={value ?? pools[0].id} onChange={(event) => onChange(Number(event.target.value))}>
      {pools.map((pool) => <option key={pool.id} value={pool.id}>{pool.name}</option>)}
    </select>
  </label>
}
