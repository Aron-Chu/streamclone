import AdminTokenGate, { AdminArchiveNav } from '../../components/admin/AdminTokenGate'
import { useArchiveJobs } from '../../hooks/useArchiveJobs'

export default function AdminJobsPage() {
  const { jobs, loading, error, refresh, retryFailed, resumeJob, cancelJob } = useArchiveJobs()

  return (
    <AdminTokenGate>
      <main className="min-h-screen bg-[#07070a] px-4 py-8 text-zinc-100">
        <div className="mx-auto max-w-5xl">
          <h1 className="text-2xl font-bold">Archive jobs</h1>
          <AdminArchiveNav />
          <button type="button" className="mb-4 rounded border border-white/15 px-3 py-1 text-sm" onClick={() => void refresh()}>
            Refresh
          </button>
          {loading ? <p className="text-sm text-zinc-400">Loading…</p> : null}
          {error ? <p className="text-sm text-red-300">{error}</p> : null}
          <div className="overflow-x-auto rounded-lg border border-white/10">
            <table className="min-w-full text-left text-sm">
              <thead className="bg-white/[0.04] text-zinc-400">
                <tr>
                  <th className="px-3 py-2">ID</th>
                  <th className="px-3 py-2">Type</th>
                  <th className="px-3 py-2">Tier</th>
                  <th className="px-3 py-2">Status</th>
                  <th className="px-3 py-2">Progress</th>
                  <th className="px-3 py-2">Actions</th>
                </tr>
              </thead>
              <tbody>
                {jobs.map((job) => (
                  <tr key={job.id} className="border-t border-white/10">
                    <td className="px-3 py-2 font-mono text-xs">{job.id.slice(0, 8)}…</td>
                    <td className="px-3 py-2">{job.jobType || '—'}</td>
                    <td className="px-3 py-2">{job.tier || '—'}</td>
                    <td className="px-3 py-2">{job.status || '—'}</td>
                    <td className="px-3 py-2">
                      {job.completedItems ?? 0}/{job.totalItems ?? 0}
                      {(job.failedItems ?? 0) > 0 ? ` (${job.failedItems} failed)` : ''}
                    </td>
                    <td className="px-3 py-2 space-x-2">
                      <button type="button" className="text-violet-300 hover:text-violet-200" onClick={() => void retryFailed(job.id)}>Retry</button>
                      <button type="button" className="text-violet-300 hover:text-violet-200" onClick={() => void resumeJob(job.id)}>Resume</button>
                      <button type="button" className="text-zinc-400 hover:text-zinc-200" onClick={() => void cancelJob(job.id)}>Cancel</button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            {jobs.length === 0 && !loading ? <p className="p-4 text-sm text-zinc-500">No jobs yet.</p> : null}
          </div>
        </div>
      </main>
    </AdminTokenGate>
  )
}
