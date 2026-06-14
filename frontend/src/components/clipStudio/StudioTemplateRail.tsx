import { useMemo, useState } from 'react'
import type { ClipperTemplate } from '../../api'
import type { FormatPreset } from './types'
import { FORMAT_OPTIONS, TEMPLATE_PREVIEW_COLORS } from './studioTheme'

interface StudioTemplateRailProps {
  templates: ClipperTemplate[]
  selectedTemplateId: string | null
  formatPreset: FormatPreset
  onApplyTemplate: (template: ClipperTemplate) => void
  onFormatPresetChange: (preset: FormatPreset) => void
}

export function StudioTemplateRail({
  templates,
  selectedTemplateId,
  formatPreset,
  onApplyTemplate,
  onFormatPresetChange,
}: StudioTemplateRailProps) {
  const [search, setSearch] = useState('')
  const [formatFilter, setFormatFilter] = useState<'all' | FormatPreset>('all')

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase()
    return templates.filter(t => {
      const matchSearch = !q
        || t.name.toLowerCase().includes(q)
        || t.id.toLowerCase().includes(q)
        || t.description.toLowerCase().includes(q)
      const matchFormat = formatFilter === 'all' || t.format_preset === formatFilter
      return matchSearch && matchFormat
    })
  }, [templates, search, formatFilter])

  return (
    <aside className="flex min-h-0 w-full flex-1 flex-col border-b border-white/[0.08] bg-[#0d0d12]/95 lg:w-[240px] lg:flex-none lg:border-b-0 lg:border-r">
      <div className="border-b border-white/[0.08] px-3 py-3">
        <p className="text-[10px] font-semibold uppercase tracking-[0.14em] text-cyan-400/90">Templates</p>
        <input
          type="search"
          placeholder="Search…"
          value={search}
          onChange={e => setSearch(e.target.value)}
          className="mt-2 w-full rounded-md border border-white/10 bg-black/30 px-2.5 py-1.5 text-xs text-zinc-100 placeholder:text-zinc-500 focus:border-cyan-500/50 focus:outline-none"
        />
        <select
          value={formatFilter}
          onChange={e => setFormatFilter(e.target.value as 'all' | FormatPreset)}
          className="mt-2 w-full rounded-md border border-white/10 bg-black/30 px-2 py-1.5 text-xs text-zinc-200 focus:border-cyan-500/50 focus:outline-none"
        >
          <option value="all">All formats</option>
          {FORMAT_OPTIONS.map(o => (
            <option key={o.id} value={o.id}>{o.hint}</option>
          ))}
        </select>
      </div>

      <div className="flex flex-wrap gap-1 border-b border-white/[0.06] px-2 py-2">
        {FORMAT_OPTIONS.map(opt => (
          <button
            key={opt.id}
            type="button"
            title={opt.hint}
            onClick={() => onFormatPresetChange(opt.id)}
            className={`rounded px-2 py-0.5 text-[10px] font-semibold transition ${
              formatPreset === opt.id
                ? 'bg-cyan-500/20 text-cyan-300 ring-1 ring-cyan-400/40'
                : 'text-zinc-500 hover:bg-white/5 hover:text-zinc-300'
            }`}
          >
            {opt.label}
          </button>
        ))}
      </div>

      <div className="flex-1 overflow-y-auto p-2">
        {filtered.length === 0 ? (
          <p className="px-1 py-4 text-center text-xs text-zinc-500">No templates match.</p>
        ) : (
          <div className="grid grid-cols-2 gap-2">
            {filtered.map(t => (
              <button
                key={t.id}
                type="button"
                title={t.description}
                onClick={() => onApplyTemplate(t)}
                className={`group flex flex-col overflow-hidden rounded-lg border text-left transition ${
                  selectedTemplateId === t.id
                    ? 'border-cyan-400/50 ring-1 ring-cyan-400/30'
                    : 'border-white/10 hover:border-white/20'
                }`}
              >
                <span
                  className="aspect-[9/16] w-full"
                  style={{
                    background: TEMPLATE_PREVIEW_COLORS[t.id] || 'linear-gradient(135deg,#27272a,#52525b)',
                  }}
                />
                <span className="truncate px-1.5 py-1 text-[10px] font-semibold text-zinc-100">{t.name}</span>
                <span className="truncate px-1.5 pb-1 text-[9px] text-zinc-500">{t.caption_preset}</span>
              </button>
            ))}
          </div>
        )}
      </div>
    </aside>
  )
}
