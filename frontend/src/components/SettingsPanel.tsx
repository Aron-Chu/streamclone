import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { getAuthDebug, getAlwaysTracked, setAlwaysTracked } from '../api'
import { useThemeEffect, useUiSettings, type ThemeName } from '../settings'
import SystemHealthPanel from './SystemHealthPanel'
import { dispatchOpenStackStatus } from '../stackStatusEvents'

const themes: Array<{ id: ThemeName; label: string }> = [
  { id: 'obsidian', label: 'Obsidian' },
  { id: 'midnight', label: 'Midnight' },
  { id: 'daylight', label: 'Daylight' },
]

export default function SettingsButton() {
  const [open, setOpen] = useState(false)
  const [newChannel, setNewChannel] = useState('')
  const [errorMsg, setErrorMsg] = useState('')
  const queryClient = useQueryClient()
  const settings = useUiSettings(s => s.settings)
  const updateSettings = useUiSettings(s => s.updateSettings)
  const authDebug = useQuery({
    queryKey: ['auth-debug'],
    queryFn: getAuthDebug,
    enabled: open,
    staleTime: 10_000,
  })
  const alwaysTracked = useQuery({
    queryKey: ['always-tracked'],
    queryFn: getAlwaysTracked,
    enabled: open,
  })
  useThemeEffect(settings.theme)

  const handleAdd = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!newChannel.trim()) return
    try {
      await setAlwaysTracked(newChannel.trim().toLowerCase(), true)
      setNewChannel('')
      setErrorMsg('')
      queryClient.invalidateQueries({ queryKey: ['always-tracked'] })
    } catch (err: any) {
      setErrorMsg(err.message || 'Failed to add channel')
    }
  }

  const handleRemove = async (login: string) => {
    try {
      await setAlwaysTracked(login, false)
      queryClient.invalidateQueries({ queryKey: ['always-tracked'] })
    } catch (err: any) {
      setErrorMsg(err.message || 'Failed to remove channel')
    }
  }

  return (
    <div className="relative z-50">
      <button
        type="button"
        aria-haspopup="dialog"
        aria-expanded={open}
        onClick={() => setOpen(value => !value)}
        className="rounded border border-white/10 bg-white/[0.06] px-3 py-2 text-xs font-black text-zinc-200 transition hover:bg-white/10"
      >
        Settings
      </button>
      {open ? (
        <div className="absolute right-0 top-11 z-50 max-h-[calc(100vh-5rem)] w-[min(22rem,calc(100vw-1rem))] overflow-y-auto rounded border border-white/10 bg-[#111117] p-4 text-zinc-100 shadow-2xl shadow-black/50">
          <div className="mb-3 flex items-center justify-between">
            <div className="text-sm font-black">Settings</div>
            <button
              type="button"
              onClick={() => setOpen(false)}
              className="rounded px-2 py-1 text-xs font-black text-zinc-400 transition hover:bg-white/10 hover:text-white"
            >
              Close
            </button>
          </div>

          <div className="space-y-4">
            <div>
              <div className="mb-2 text-[11px] font-black uppercase text-zinc-500">Twitch Connect</div>
              <div className="rounded border border-white/10 bg-white/[0.035] p-3">
                {authDebug.isLoading ? (
                  <div className="text-xs font-bold text-zinc-400">Checking auth config</div>
                ) : authDebug.data ? (
                  <div className="space-y-2">
                    <div className="flex flex-wrap gap-1.5">
                      <StatusChip label={authDebug.data.ready ? 'Twitch app ready' : 'Twitch app missing'} good={authDebug.data.ready} />
                      <StatusChip label={authDebug.data.frontendMatchesRequest ? 'Frontend origin ok' : 'Frontend mismatch'} good={authDebug.data.frontendMatchesRequest} />
                      <StatusChip label={`SameSite ${authDebug.data.cookieSameSite || 'lax'}`} good={authDebug.data.cookieSameSite !== 'none' || authDebug.data.cookieSecureOnThisOrigin} />
                    </div>
                    <div className="space-y-1 text-[11px] font-semibold text-zinc-500">
                      <div className="truncate" title={authDebug.data.frontendUrl}>Frontend {authDebug.data.frontendUrl || '-'}</div>
                    </div>
                    {(authDebug.data.warnings ?? []).length ? (
                      <div className="rounded border border-amber-300/20 bg-amber-400/10 p-2 text-xs font-semibold text-amber-100">
                        {authDebug.data.warnings?.[0]}
                      </div>
                    ) : (
                      <div className="text-xs font-semibold text-emerald-200">Local auth config looks consistent.</div>
                    )}
                  </div>
                ) : (
                  <div className="text-xs font-bold text-red-200">Auth debug endpoint is unavailable.</div>
                )}
              </div>
            </div>

            <div>
              <div className="mb-2 text-[11px] font-black uppercase text-zinc-500">Preview</div>
              <div className="grid grid-cols-2 gap-2">
                <Toggle
                  label="Autoplay"
                  checked={settings.previewAutoplay}
                  onChange={value => updateSettings({ previewAutoplay: value })}
                />
                <Toggle
                  label="Muted"
                  checked={settings.previewMuted}
                  onChange={value => updateSettings({ previewMuted: value })}
                />
              </div>
            </div>

            <div>
              <div className="mb-2 text-[11px] font-black uppercase text-zinc-500">Channel layout</div>
              <div className="grid grid-cols-2 gap-2">
                <button
                  type="button"
                  onClick={() => updateSettings({ bottomDensity: 'comfortable' })}
                  className={`rounded border px-2 py-2 text-xs font-black transition ${
                    settings.bottomDensity === 'comfortable' ? 'border-violet-300 bg-white text-zinc-950' : 'border-white/10 bg-white/[0.04] text-zinc-300 hover:bg-white/10'
                  }`}
                >
                  Comfortable
                </button>
                <button
                  type="button"
                  onClick={() => updateSettings({ bottomDensity: 'dense' })}
                  className={`rounded border px-2 py-2 text-xs font-black transition ${
                    settings.bottomDensity === 'dense' ? 'border-violet-300 bg-white text-zinc-950' : 'border-white/10 bg-white/[0.04] text-zinc-300 hover:bg-white/10'
                  }`}
                >
                  Dense
                </button>
              </div>
            </div>

            <div>
              <div className="mb-2 text-[11px] font-black uppercase text-zinc-500">Theme</div>
              <div className="grid grid-cols-3 gap-2">
                {themes.map(theme => (
                  <button
                    key={theme.id}
                    type="button"
                    onClick={() => updateSettings({ theme: theme.id })}
                    className={`rounded border px-2 py-2 text-xs font-black transition ${
                      settings.theme === theme.id ? 'border-violet-300 bg-white text-zinc-950' : 'border-white/10 bg-white/[0.04] text-zinc-300 hover:bg-white/10'
                    }`}
                  >
                    {theme.label}
                  </button>
                ))}
              </div>
            </div>

            <div>
              <div className="mb-2 text-[11px] font-black uppercase text-zinc-500">Rail</div>
              <div className="grid grid-cols-2 gap-2">
                <Toggle label="Live" checked={settings.railSections.live} onChange={value => updateSettings({ railSections: { live: value } })} />
                <Toggle label="Offline" checked={settings.railSections.offline} onChange={value => updateSettings({ railSections: { offline: value } })} />
                <Toggle label="Top" checked={settings.railSections.top} onChange={value => updateSettings({ railSections: { top: value } })} />
                <Toggle label="Categories" checked={settings.railSections.categories} onChange={value => updateSettings({ railSections: { categories: value } })} />
              </div>
            </div>

            <div>
              <div className="mb-2 text-[11px] font-black uppercase text-zinc-500">Emotes</div>
              <Toggle
                label="Auto-load"
                checked={settings.emoteAutoLoad}
                onChange={value => updateSettings({ emoteAutoLoad: value })}
              />
            </div>

            <div>
              <div className="mb-2 text-[11px] font-black uppercase text-zinc-500">Always-Tracked Channels</div>
              <div className="rounded border border-white/10 bg-white/[0.035] p-3 space-y-3">
                <form onSubmit={handleAdd} className="flex gap-2">
                  <input
                    type="text"
                    placeholder="Channel login (e.g. xqc)"
                    value={newChannel}
                    onChange={e => setNewChannel(e.target.value)}
                    className="flex-1 rounded border border-white/10 bg-black/40 px-2 py-1.5 text-xs text-white placeholder-zinc-500 focus:border-violet-500 focus:outline-none"
                  />
                  <button
                    type="submit"
                    className="rounded bg-violet-600 px-3 py-1.5 text-xs font-black text-white hover:bg-violet-700 transition"
                  >
                    Add
                  </button>
                </form>
                {errorMsg && (
                  <div className="text-[10px] font-bold text-red-400">{errorMsg}</div>
                )}
                {alwaysTracked.isLoading ? (
                  <div className="text-xs text-zinc-500">Loading channels...</div>
                ) : alwaysTracked.data?.channels?.length ? (
                  <div className="max-h-28 overflow-y-auto space-y-1.5 pr-1">
                    {alwaysTracked.data.channels.map(ch => (
                      <div key={ch} className="flex items-center justify-between rounded bg-white/[0.03] px-2 py-1 text-xs">
                        <span className="font-bold text-zinc-200">{ch}</span>
                        <button
                          type="button"
                          onClick={() => handleRemove(ch)}
                          className="text-[10px] font-black text-red-400 hover:text-red-300 transition"
                        >
                          Remove
                        </button>
                      </div>
                    ))}
                  </div>
                ) : (
                  <div className="text-xs text-zinc-500 italic">No channels tracked yet.</div>
                )}
              </div>
            </div>
            <div>
              <div className="mb-2 text-[11px] font-black uppercase text-zinc-500">System status</div>
              <div className="rounded border border-white/10 bg-white/[0.035] p-3 space-y-3">
                <SystemHealthPanel variant="compact" />
                <Link
                  to="/network"
                  onClick={() => setOpen(false)}
                  className="block w-full rounded border border-white/10 bg-white/[0.04] px-3 py-2 text-center text-xs font-black text-zinc-200 transition hover:bg-white/10"
                >
                  Network monitor
                </Link>
                <button
                  type="button"
                  onClick={() => {
                    setOpen(false)
                    dispatchOpenStackStatus()
                  }}
                  className="w-full rounded border border-violet-400/30 bg-violet-500/10 px-3 py-2 text-xs font-black text-violet-100 transition hover:bg-violet-500/20"
                >
                  Open full status
                </button>
              </div>
            </div>
          </div>
        </div>
      ) : null}
    </div>
  )
}

function StatusChip({ label, good }: { label: string; good: boolean }) {
  return (
    <span className={`rounded px-2 py-1 text-[10px] font-black uppercase ${good ? 'bg-emerald-400/15 text-emerald-100' : 'bg-amber-400/15 text-amber-100'}`}>
      {label}
    </span>
  )
}

function Toggle({ label, checked, onChange }: { label: string; checked: boolean; onChange: (value: boolean) => void }) {
  return (
    <button
      type="button"
      aria-pressed={checked}
      onClick={() => onChange(!checked)}
      className={`flex items-center justify-between gap-2 rounded border px-2 py-2 text-xs font-black transition ${
        checked ? 'border-emerald-300/40 bg-emerald-400/15 text-emerald-100' : 'border-white/10 bg-white/[0.04] text-zinc-400 hover:bg-white/10 hover:text-white'
      }`}
    >
      <span>{label}</span>
      <span className={`h-2.5 w-2.5 rounded-full ${checked ? 'bg-emerald-300' : 'bg-zinc-600'}`} />
    </button>
  )
}
