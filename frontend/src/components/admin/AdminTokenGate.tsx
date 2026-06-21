import { FormEvent, useState } from 'react'
import { Link } from 'react-router-dom'
import { ADMIN_ARCHIVE_UI_ALLOWED, ADMIN_ARCHIVE_UI_ENABLED } from '../../config'
import { getAdminArchiveToken, setAdminArchiveToken } from '../../hooks/useArchiveJobs'

type Props = {
  children: React.ReactNode
}

export default function AdminTokenGate({ children }: Props) {
  const [token, setToken] = useState(getAdminArchiveToken())
  const [saved, setSaved] = useState(Boolean(getAdminArchiveToken()))

  if (!ADMIN_ARCHIVE_UI_ENABLED && !ADMIN_ARCHIVE_UI_ALLOWED) {
    return <AdminCLIHelp reason="Admin UI is disabled on this deployment profile." />
  }

  if (!ADMIN_ARCHIVE_UI_ALLOWED) {
    return <AdminCLIHelp reason="Archive admin UI is disabled over plain HTTP on non-localhost origins." />
  }

  if (!saved) {
    return (
      <main className="min-h-screen bg-[#07070a] px-4 py-10 text-zinc-100">
        <div className="mx-auto max-w-lg rounded-lg border border-white/10 bg-white/[0.04] p-6">
          <h1 className="text-xl font-bold">Archive admin token</h1>
          <p className="mt-2 text-sm text-zinc-400">
            Paste your operator <code className="text-zinc-200">ADMIN_ARCHIVE_TOKEN</code>. It is stored in
            sessionStorage only for this browser tab session — never in config.js.
          </p>
          <form
            className="mt-4 space-y-3"
            onSubmit={(e: FormEvent) => {
              e.preventDefault()
              setAdminArchiveToken(token)
              setSaved(true)
            }}
          >
            <input
              type="password"
              autoComplete="off"
              className="w-full rounded border border-white/15 bg-black/40 px-3 py-2 text-sm"
              placeholder="Admin archive token"
              value={token}
              onChange={(e) => setToken(e.target.value)}
            />
            <button type="submit" className="rounded bg-violet-600 px-4 py-2 text-sm font-semibold hover:bg-violet-500">
              Save for session
            </button>
          </form>
          <p className="mt-4 text-xs text-zinc-500">
            Prefer CLI on production hosts: <code>docker compose exec analytics-workers go run ./cmd/backfill jobs list</code>
          </p>
        </div>
      </main>
    )
  }

  return <>{children}</>
}

function AdminCLIHelp({ reason }: { reason: string }) {
  return (
    <main className="min-h-screen bg-[#07070a] px-4 py-10 text-zinc-100">
      <div className="mx-auto max-w-2xl rounded-lg border border-amber-400/20 bg-amber-500/5 p-6">
        <h1 className="text-xl font-bold text-amber-100">Archive admin — CLI only</h1>
        <p className="mt-2 text-sm text-zinc-300">{reason}</p>
        <ul className="mt-4 list-disc space-y-2 pl-5 text-sm text-zinc-300">
          <li>List jobs: <code>go run ./cmd/backfill jobs list</code></li>
          <li>Coverage: <code>go run ./cmd/backfill coverage report</code></li>
          <li>SSH tunnel Grafana: <code>ssh -L 3000:127.0.0.1:3000 user@host</code></li>
        </ul>
        <p className="mt-4 text-xs text-zinc-500">
          Local dev: use localhost or HTTPS to enable the token gate UI.
        </p>
        <Link to="/" className="mt-6 inline-block text-sm text-violet-300 hover:text-violet-200">← Back to directory</Link>
      </div>
    </main>
  )
}

export function AdminArchiveNav() {
  return (
    <nav className="mb-6 flex flex-wrap gap-3 text-sm">
      <Link className="text-violet-300 hover:text-violet-200" to="/admin/archive">Overview</Link>
      <Link className="text-violet-300 hover:text-violet-200" to="/admin/jobs">Jobs</Link>
      <Link className="text-violet-300 hover:text-violet-200" to="/admin/coverage">Coverage</Link>
      <button
        type="button"
        className="text-zinc-400 hover:text-zinc-200"
        onClick={() => {
          setAdminArchiveToken('')
          window.location.reload()
        }}
      >
        Clear token
      </button>
    </nav>
  )
}
