import { useEffect, useState } from 'react'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { PULSE_WIRE_ENABLED } from '../../config'
import {
  fetchPulseWireStreamer,
  PulseWireApiError,
  type PulseWireStreamerProfile,
  type PulseWireWindow,
} from '../../pulseWireApi'
import { WINDOW_OPTIONS } from './PulseWireFilters'
import { DirectoryLayout } from '../directory/DirectoryLayout'
import SocialSpreadPanel from '../channel/SocialSpreadPanel'
import {
  deltaTone,
  formatDeltaPct,
  formatRankDelta,
  formatSince,
  formatViewers,
  windowShortLabel,
} from '../../utils/pulseWireFormat'
import ViewerSparkline from './ViewerSparkline'
import StoryCompactCard from './StoryCompactCard'

function parseWindow(raw: string | null): PulseWireWindow {
  if (raw === 'today') return 'today'
  if (raw === '7d') return '7d'
  return '24h'
}

function StatCard({ label, value, tone, hint }: { label: string; value: string; tone?: string; hint?: string }) {
  return (
    <div className="rounded-xl border border-[#26262C] bg-[#161619] px-4 py-3">
      <p className="text-[11px] font-bold uppercase tracking-[0.06em] text-[#7A7A85]">{label}</p>
      <p className={`mt-1 text-2xl font-black ${tone ?? 'text-[#F7F7F8]'}`}>{value}</p>
      {hint ? <p className="text-[11px] text-[#7A7A85]">{hint}</p> : null}
    </div>
  )
}

function ProfileSkeleton() {
  return (
    <div className="space-y-6 animate-pulse">
      <div className="h-24 rounded-2xl border border-[#2A2A2E] bg-[#121217]" />
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        {Array.from({ length: 4 }).map((_, index) => (
          <div key={index} className="h-20 rounded-xl border border-[#2A2A2E] bg-[#121217]" />
        ))}
      </div>
      <div className="h-40 rounded-2xl border border-[#2A2A2E] bg-[#121217]" />
    </div>
  )
}

export default function StreamerStatProfile() {
  const { login = '' } = useParams<{ login: string }>()
  const [searchParams, setSearchParams] = useSearchParams()
  const windowRange = parseWindow(searchParams.get('window'))
  const [profile, setProfile] = useState<PulseWireStreamerProfile | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [disabledHint, setDisabledHint] = useState(PULSE_WIRE_ENABLED ? '' : 'Set PULSE_WIRE_ENABLED=true in .env and restart Streamclone.')

  useEffect(() => {
    if (!PULSE_WIRE_ENABLED || !login) {
      setLoading(false)
      return
    }
    let cancelled = false
    setLoading(true)
    setError('')
    setDisabledHint('')
    fetchPulseWireStreamer(login, { window: windowRange })
      .then(res => {
        if (!cancelled) setProfile(res)
      })
      .catch(err => {
        if (cancelled) return
        if (err instanceof PulseWireApiError && err.code === 'pulse_wire_disabled') {
          setDisabledHint(err.hint ?? 'Set PULSE_WIRE_ENABLED=true in .env and restart Streamclone.')
          return
        }
        setError(err instanceof Error ? err.message : 'Failed to load streamer profile')
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [login, windowRange])

  const setWindow = (next: PulseWireWindow) => {
    setSearchParams(prev => {
      const params = new URLSearchParams(prev)
      if (next === '24h') params.delete('window')
      else params.set('window', next)
      return params
    })
  }

  const displayName = profile?.displayName || login
  const initial = (displayName || '?').slice(0, 1).toUpperCase()
  const backSearch = searchParams.toString()
  const backSuffix = backSearch ? `?${backSearch}` : ''

  return (
    <DirectoryLayout headerSubtitle="Pulse Wire" showBrowseLink showPulseWireLink={PULSE_WIRE_ENABLED} pulseWireActive>
      <div className="space-y-6">
        <Link
          to={`/pulse-wire${backSuffix}`}
          className="inline-flex items-center gap-1 text-sm font-semibold text-[#ADADB8] transition hover:text-[#EFEFF1] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#A970FF]"
        >
          ← Back to Pulse Wire
        </Link>

        {disabledHint ? (
          <p className="rounded-lg border border-amber-400/30 bg-amber-500/10 p-3 text-sm text-amber-100">
            Pulse Wire is disabled on this install. {disabledHint}
          </p>
        ) : null}
        {error ? <p className="rounded-lg border border-red-400/30 bg-red-500/10 p-3 text-sm text-red-100">{error}</p> : null}

        {loading ? <ProfileSkeleton /> : null}

        {!loading && !disabledHint ? (
          <>
            <header className="flex flex-wrap items-center gap-4 rounded-2xl border border-[#2A2A2E] bg-[#161619] p-5">
              {profile?.avatarUrl ? (
                <img src={profile.avatarUrl} alt="" className="h-16 w-16 shrink-0 rounded-full object-cover" loading="lazy" />
              ) : (
                <span className="grid h-16 w-16 shrink-0 place-items-center rounded-full bg-gradient-to-br from-[#9147FF] to-[#5A2BAE] text-2xl font-black text-white">
                  {initial}
                </span>
              )}
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <h1 className="text-[26px] font-bold text-[#F7F7F8]">{displayName}</h1>
                  {profile?.isLive ? (
                    <span className="rounded-full bg-[#2A1515] px-2 py-0.5 text-[11px] font-bold uppercase tracking-wide text-[#FF5C57]">
                      Live
                    </span>
                  ) : null}
                  {profile?.newEntrant ? (
                    <span className="rounded-full bg-[#16321F] px-2 py-0.5 text-[11px] font-bold uppercase tracking-wide text-[#3FCB7E]">
                      New entrant
                    </span>
                  ) : null}
                </div>
                <p className="text-sm text-[#ADADB8]">
                  @{login}
                  {profile?.category ? <> · {profile.category}</> : null}
                  {profile?.rankNow != null ? <> · rank #{profile.rankNow}</> : null}
                </p>
                <p className="mt-1 text-[11px] font-semibold text-[#7A7A85]">
                  {formatSince(profile?.since, windowRange)} · {windowShortLabel(windowRange)} stats
                </p>
              </div>
              <div className="flex shrink-0 flex-wrap gap-2">
                {WINDOW_OPTIONS.map(option => {
                  const active = windowRange === option.id
                  return (
                    <button
                      key={option.id}
                      type="button"
                      onClick={() => setWindow(option.id)}
                      aria-pressed={active}
                      className={`rounded-full border px-3 py-1.5 text-xs font-semibold transition focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#A970FF] ${
                        active
                          ? 'border-[#A970FF] bg-[#9147FF]/20 text-[#EFEFF1]'
                          : 'border-[#2A2A2E] bg-[#1B1B1F] text-[#ADADB8] hover:border-[#3A3A40] hover:text-[#EFEFF1]'
                      }`}
                    >
                      {option.label}
                    </button>
                  )
                })}
                <Link
                  to={`/c/${encodeURIComponent(login)}`}
                  className="rounded-lg border border-[#33333A] bg-[#1F1F23] px-4 py-2 text-sm font-semibold text-[#EFEFF1] transition hover:border-[#A970FF]/40 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#A970FF]"
                >
                  View channel
                </Link>
              </div>
            </header>

            <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
              <StatCard
                label="Viewers now"
                value={formatViewers(profile?.viewersNow)}
                hint={profile?.viewersPrev != null ? `was ${formatViewers(profile.viewersPrev)}` : undefined}
              />
              <StatCard
                label="Viewer change"
                value={formatDeltaPct(profile?.viewerDeltaPct)}
                tone={deltaTone(profile?.viewerDeltaPct)}
              />
              <StatCard
                label="Rank change"
                value={formatRankDelta(profile?.rankDelta)}
                tone={deltaTone(profile?.rankDelta)}
                hint={profile?.rankNow != null ? `now #${profile.rankNow}` : undefined}
              />
              <StatCard
                label="Followers"
                value={profile?.followerSampled ? formatViewers(profile?.followersNow) : '—'}
                tone={profile?.followerSampled ? deltaTone(profile?.followerDelta) : undefined}
                hint={
                  profile?.followerSampled
                    ? profile?.followerDelta != null
                      ? `${profile.followerDelta > 0 ? '+' : ''}${formatViewers(Math.abs(profile.followerDelta))} sampled`
                      : 'sampled'
                    : 'not sampled'
                }
              />
            </div>

            <section className="rounded-2xl border border-[#2A2A2E] bg-[#161619] p-5">
              <div className="mb-3 flex items-center justify-between gap-2">
                <h2 className="text-[15px] font-bold text-[#F7F7F8]">Viewer trend</h2>
                <span className="text-[11px] font-semibold text-[#7A7A85]">
                  {profile?.viewerSeries?.length ? `${profile.viewerSeries.length} samples` : 'gathering data'}
                </span>
              </div>
              {profile?.viewerSeries && profile.viewerSeries.length >= 2 ? (
                <ViewerSparkline
                  points={profile.viewerSeries}
                  width={640}
                  height={120}
                  className="h-28 w-full"
                  ariaLabel={`${displayName} viewer trend`}
                />
              ) : (
                <p className="text-xs text-[#7A7A85]">
                  No viewer history sampled for this streamer in {windowShortLabel(windowRange)} yet. The directory sampler builds this series over time.
                </p>
              )}
              {profile?.clipVelocity != null || profile?.risingScore != null ? (
                <div className="mt-4 flex flex-wrap gap-4 text-xs text-[#ADADB8]">
                  {profile?.risingScore != null ? (
                    <span>
                      Rising score <span className="font-bold text-[#EFEFF1]">{Math.round(profile.risingScore)}</span>
                    </span>
                  ) : null}
                  {profile?.clipVelocity != null ? (
                    <span>
                      Clip velocity <span className="font-bold text-[#EFEFF1]">{Math.round(profile.clipVelocity)}</span>
                    </span>
                  ) : null}
                </div>
              ) : null}
            </section>

            <section className="rounded-2xl border border-[#2A2A2E] bg-[#161619] p-5">
              <h2 className="mb-3 text-[15px] font-bold text-[#F7F7F8]">Recent stories</h2>
              {profile?.recentStories?.length ? (
                <div className="space-y-3">
                  {profile.recentStories.slice(0, 6).map(story => (
                    <StoryCompactCard
                      key={story.story.id}
                      story={story}
                      variant="channel"
                      detailSearch={backSuffix}
                    />
                  ))}
                </div>
              ) : login ? (
                <SocialSpreadPanel login={login} />
              ) : null}
            </section>
          </>
        ) : null}
      </div>
    </DirectoryLayout>
  )
}
