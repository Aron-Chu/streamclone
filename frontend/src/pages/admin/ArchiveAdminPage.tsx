import AdminTokenGate, { AdminArchiveNav } from '../../components/admin/AdminTokenGate'
import { useArchiveCoverage, useArchiveJobs } from '../../hooks/useArchiveJobs'

export default function ArchiveAdminPage() {
  const jobs = useArchiveJobs()
  const coverage = useArchiveCoverage()

  return (
    <AdminTokenGate>
      <main className="min-h-screen bg-[#07070a] px-4 py-8 text-zinc-100">
        <div className="mx-auto max-w-5xl">
          <h1 className="text-2xl font-bold">Archive overview</h1>
          <AdminArchiveNav />
          <div className="grid gap-4 sm:grid-cols-3">
            <StatCard label="Active jobs" value={jobs.jobs.filter((j) => (j.status ?? '') === 'running').length} />
            <StatCard label="Queued jobs" value={jobs.jobs.filter((j) => (j.status ?? '') === 'queued').length} />
            <StatCard label="Streams tracked" value={coverage.report?.streams?.total ?? '—'} />
          </div>
          {jobs.error ? <p className="mt-4 text-sm text-red-300">{jobs.error}</p> : null}
          <button type="button" className="mt-4 rounded border border-white/15 px-3 py-1 text-sm" onClick={() => { void jobs.refresh(); void coverage.refresh() }}>
            Refresh
          </button>
        </div>
      </main>
    </AdminTokenGate>
  )
}

function StatCard({ label, value }: { label: string; value: string | number }) {
  return (
    <div className="rounded-lg border border-white/10 bg-white/[0.04] p-4">
      <div className="text-xs uppercase tracking-wide text-zinc-500">{label}</div>
      <div className="mt-1 text-2xl font-bold">{value}</div>
    </div>
  )
}
