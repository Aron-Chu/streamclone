import { useMemo, useState } from 'react'
import type { HostNetworkContainerStats } from '../../api'

type SortKey = 'name' | 'rxBytes' | 'txBytes' | 'cpuPerc'

function sortValue(row: HostNetworkContainerStats, key: SortKey) {
  switch (key) {
    case 'rxBytes':
      return row.rxBytes ?? -1
    case 'txBytes':
      return row.txBytes ?? -1
    case 'cpuPerc':
      return Number.parseFloat(String(row.cpuPerc ?? '').replace('%', '')) || -1
    default:
      return row.name
  }
}

export interface ContainerNetworkTableProps {
  containers: HostNetworkContainerStats[]
  loading?: boolean
}

export default function ContainerNetworkTable({ containers, loading = false }: ContainerNetworkTableProps) {
  const [sortKey, setSortKey] = useState<SortKey>('txBytes')
  const [sortDesc, setSortDesc] = useState(true)

  const rows = useMemo(() => {
    const next = [...containers]
    next.sort((a, b) => {
      const av = sortValue(a, sortKey)
      const bv = sortValue(b, sortKey)
      if (typeof av === 'string' && typeof bv === 'string') {
        return sortDesc ? bv.localeCompare(av) : av.localeCompare(bv)
      }
      return sortDesc ? Number(bv) - Number(av) : Number(av) - Number(bv)
    })
    return next
  }, [containers, sortDesc, sortKey])

  const toggleSort = (key: SortKey) => {
    if (sortKey === key) {
      setSortDesc(value => !value)
      return
    }
    setSortKey(key)
    setSortDesc(key !== 'name')
  }

  const headerClass = (key: SortKey) =>
    `cursor-pointer pb-2 pr-4 font-black uppercase transition hover:text-zinc-300 ${sortKey === key ? 'text-violet-200' : 'text-zinc-500'}`

  return (
    <section className="rounded-xl border border-white/10 bg-[#0e0e10] p-4">
      <div className="mb-3 text-[11px] font-black uppercase tracking-wide text-zinc-500">Container network</div>
      {loading && !containers.length ? (
        <div className="text-sm font-semibold text-zinc-500">Loading Docker stats…</div>
      ) : !containers.length ? (
        <div className="text-sm font-semibold text-zinc-500">Install helper unavailable or no streamclone containers running.</div>
      ) : (
        <div className="overflow-x-auto">
          <table className="min-w-full text-left text-xs">
            <thead>
              <tr>
                <th className={headerClass('name')} onClick={() => toggleSort('name')}>Name</th>
                <th className={headerClass('rxBytes')} onClick={() => toggleSort('rxBytes')}>Net rx</th>
                <th className={headerClass('txBytes')} onClick={() => toggleSort('txBytes')}>Net tx</th>
                <th className={headerClass('cpuPerc')} onClick={() => toggleSort('cpuPerc')}>CPU</th>
                <th className="pb-2 font-black uppercase text-zinc-500">Memory</th>
              </tr>
            </thead>
            <tbody>
              {rows.map(row => (
                <tr key={row.name} className="border-t border-white/5 text-zinc-300">
                  <td className="py-2 pr-4 font-semibold text-white">{row.name}</td>
                  <td className="py-2 pr-4 font-mono">{row.rxHuman || '—'}</td>
                  <td className="py-2 pr-4 font-mono">{row.txHuman || '—'}</td>
                  <td className="py-2 pr-4">{row.cpuPerc || '—'}</td>
                  <td className="py-2">{row.memUsage || '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  )
}
