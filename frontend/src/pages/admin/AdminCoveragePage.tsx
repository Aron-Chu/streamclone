import AdminTokenGate, { AdminArchiveNav } from '../../components/admin/AdminTokenGate'
import { useArchiveCoverage } from '../../hooks/useArchiveJobs'

export default function AdminCoveragePage() {
  const { report, loading, error, refresh } = useArchiveCoverage()

  return (
    <AdminTokenGate>
      <main className="min-h-screen bg-[#07070a] px-4 py-8 text-zinc-100">
        <div className="mx-auto max-w-5xl">
          <h1 className="text-2xl font-bold">Archive coverage</h1>
          <AdminArchiveNav />
          <button type="button" className="mb-4 rounded border border-white/15 px-3 py-1 text-sm" onClick={() => void refresh()}>
            Refresh
          </button>
          {loading ? <p className="text-sm text-zinc-400">Loading…</p> : null}
          {error ? <p className="text-sm text-red-300">{error}</p> : null}
          {report ? (
            <div className="grid gap-4 sm:grid-cols-2">
              <Card title="Roster">
                <Row label="Top N" value={report.roster?.topN} />
                <Row label="Tracked" value={report.roster?.totalTracked} />
              </Card>
              <Card title="Streams">
                <Row label="Total" value={report.streams?.total} />
                <Row label="Live good" value={report.streams?.liveGood} />
                <Row label="Partial" value={report.streams?.partial} />
                <Row label="TT required" value={report.streams?.ttRequired} />
              </Card>
              <Card title="Backfill queue">
                {(report.backfillJobs || []).map((row) => (
                  <Row key={`${row.tier}-${row.status}`} label={`${row.tier} / ${row.status}`} value={row.count} />
                ))}
              </Card>
            </div>
          ) : null}
        </div>
      </main>
    </AdminTokenGate>
  )
}

function Card({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="rounded-lg border border-white/10 bg-white/[0.04] p-4">
      <h2 className="font-semibold text-violet-200">{title}</h2>
      <div className="mt-3 space-y-1 text-sm">{children}</div>
    </div>
  )
}

function Row({ label, value }: { label: string; value?: number }) {
  return (
    <div className="flex justify-between gap-4">
      <span className="text-zinc-400">{label}</span>
      <span>{value ?? '—'}</span>
    </div>
  )
}
