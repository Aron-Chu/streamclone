const SOURCE_META: Record<string, { label: string; color: string }> = {
  reddit_thread: { label: 'Reddit / LSF', color: '#FF4500' },
  x_post: { label: 'X', color: '#1D9BF0' },
  tiktok_video: { label: 'TikTok', color: '#00D7B0' },
  youtube_video: { label: 'YouTube Shorts', color: '#FF0000' },
  twitch_clip: { label: 'Twitch clips', color: '#9147FF' },
}

export default function SourceMixPanel({ mix, windowLabel = '24h' }: { mix: Record<string, number>; windowLabel?: string }) {
  const entries = Object.entries(mix).sort((a, b) => b[1] - a[1])
  const total = entries.reduce((sum, [, count]) => sum + count, 0)
  return (
    <div className="rounded-xl border border-[#2A2A2E] bg-[#161619] p-4">
      <h3 className="mb-3 text-[15px] font-bold text-[#F7F7F8]">Source mix · {windowLabel}</h3>
      {!entries.length ? <p className="text-xs text-[#7A7A85]">No evidence mix available yet.</p> : null}
      {entries.length ? (
        <>
          <div className="mb-3 flex h-2 overflow-hidden rounded-full bg-[#1B1B1F]">
            {entries.map(([source, count]) => {
              const pct = total > 0 ? (count / total) * 100 : 0
              return (
                <div
                  key={`bar-${source}`}
                  className="h-full"
                  style={{ width: `${pct}%`, backgroundColor: SOURCE_META[source]?.color ?? '#7A7A85' }}
                />
              )
            })}
          </div>
          <ul className="space-y-2 text-sm">
            {entries.map(([source, count]) => {
              const pct = total > 0 ? Math.round((100 * count) / total) : 0
              const meta = SOURCE_META[source] ?? { label: source.replace(/_/g, ' '), color: '#7A7A85' }
              return (
                <li key={source} className="flex items-center justify-between gap-3 text-[#ADADB8]">
                  <span className="inline-flex items-center gap-2">
                    <span className="h-2.5 w-2.5 rounded-full" style={{ backgroundColor: meta.color }} />
                    {meta.label}
                  </span>
                  <span className="font-semibold text-[#D6D6DE]">{pct}%</span>
                </li>
              )
            })}
          </ul>
        </>
      ) : null}
    </div>
  )
}
