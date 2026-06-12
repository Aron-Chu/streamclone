import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import {
  describeClipperJobState,
  getClipperJobs,
  type ClipperJob,
} from '../api'
import OptionalServicesPanel from './OptionalServicesPanel'
import StackStatusButton from './StackStatusButton'

function JobRow({ job }: { job: ClipperJob }) {
  const stateLabel = describeClipperJobState(job)
  const ctx = job.moment_context
  const when = ctx?.minute_ts
    ? new Date(ctx.minute_ts).toLocaleString([], { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' })
    : new Date(job.created_at).toLocaleString([], { month: 'short', day: 'numeric' })

  return (
    <Link
      to={`/studio/${job.id}`}
      className="flex items-center gap-3 rounded-lg border border-white/10 bg-white/[0.03] p-3 transition hover:border-cyan-500/30 hover:bg-white/[0.06]"
    >
      <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-violet-500/15 text-sm font-bold text-violet-300">
        {job.channel.charAt(0).toUpperCase()}
      </span>
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-semibold text-zinc-100">{job.title || `${job.channel} clip`}</p>
        <p className="truncate text-xs text-zinc-500">{job.channel} · {when}</p>
      </div>
      <span className={`shrink-0 rounded-full px-2 py-0.5 text-[10px] font-semibold ${
        job.state === 'ready' ? 'bg-emerald-500/15 text-emerald-300'
        : job.state === 'failed' ? 'bg-rose-500/15 text-rose-300'
        : 'bg-cyan-500/15 text-cyan-300'
      }`}>
        {stateLabel}
      </span>
    </Link>
  )
}

export default function ClipStudioIndex() {
  const jobsQuery = useQuery({
    queryKey: ['clipper-jobs-archive'],
    queryFn: () => getClipperJobs(50),
    retry: 1,
  })

  const jobs = jobsQuery.data?.items ?? []
  const loadFailed = jobsQuery.isError

  return (
    <main className="min-h-[calc(100vh-56px)] bg-[#0d0d12] text-zinc-100">
      <div className="pointer-events-none fixed inset-0 bg-[radial-gradient(ellipse_at_top,rgba(34,211,238,0.08),transparent_50%)]" />
      <div className="relative mx-auto max-w-3xl px-4 py-8">
        <div className="mb-6 flex flex-wrap items-center gap-3">
          <StackStatusButton />
          <Link to="/" className="text-xs text-zinc-400 hover:text-zinc-200">Live directory</Link>
        </div>

        <p className="text-[10px] font-semibold uppercase tracking-[0.16em] text-cyan-400/90">Clip Studio</p>
        <h1 className="mt-1 text-2xl font-bold text-zinc-50">Local clip archive</h1>
        <p className="mt-2 max-w-xl text-sm text-zinc-400">
          Vertical exports from analytics moments and manual triggers. Clip Studio is an optional tier — start the clipper profile to create and edit clips.
        </p>

        <div className="mt-6">
          <OptionalServicesPanel variant="banner" focus="clipper" />
        </div>

        <div className="mt-8">
          <h2 className="mb-3 text-xs font-semibold uppercase tracking-wider text-zinc-500">Recent jobs</h2>
          {jobsQuery.isLoading ? (
            <p className="text-sm text-zinc-500">Loading archive…</p>
          ) : loadFailed ? (
            <div className="rounded-lg border border-rose-500/20 bg-rose-500/5 p-4 text-sm text-rose-200">
              Could not reach the clipper service. Use Start Clip Studio above, then refresh.
            </div>
          ) : jobs.length === 0 ? (
            <div className="rounded-lg border border-white/10 bg-white/[0.03] p-6 text-center text-sm text-zinc-500">
              No clips yet. Queue a moment from Analytics or trigger a manual clip when Clip Studio is running.
            </div>
          ) : (
            <div className="flex flex-col gap-2">
              {jobs.map(job => (
                <JobRow key={job.id} job={job} />
              ))}
            </div>
          )}
        </div>
      </div>
    </main>
  )
}
