import { useCallback, useEffect, useState } from 'react'
import { ANALYTICS } from '../config'

const TOKEN_KEY = 'adminArchiveToken'

export function getAdminArchiveToken(): string {
  try {
    return sessionStorage.getItem(TOKEN_KEY) || ''
  } catch {
    return ''
  }
}

export function setAdminArchiveToken(token: string) {
  try {
    if (token.trim()) {
      sessionStorage.setItem(TOKEN_KEY, token.trim())
    } else {
      sessionStorage.removeItem(TOKEN_KEY)
    }
  } catch {
    // ignore
  }
}

async function adminFetch<T>(path: string, init: RequestInit = {}): Promise<T> {
  const token = getAdminArchiveToken()
  const headers = new Headers(init.headers)
  if (token) {
    headers.set('X-Admin-Archive-Token', token)
  }
  headers.set('Accept', 'application/json')
  const res = await fetch(`${ANALYTICS}${path}`, { ...init, headers })
  if (!res.ok) {
    const body = await res.text()
    throw new Error(body || `HTTP ${res.status}`)
  }
  return res.json() as Promise<T>
}

export type ArchiveJobRow = {
  id: string
  jobType?: string
  tier?: string
  status?: string
  totalItems?: number
  completedItems?: number
  failedItems?: number
  heartbeatAt?: string
  updatedAt?: string
}

export type CoverageReport = {
  generatedAt?: string
  roster?: { topN?: number; totalTracked?: number }
  streams?: { total?: number; liveGood?: number; partial?: number; ttRequired?: number }
  backfillJobs?: Array<{ tier: string; status: string; count: number }>
}

export function useArchiveJobs() {
  const [jobs, setJobs] = useState<ArchiveJobRow[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const data = await adminFetch<{ jobs: ArchiveJobRow[] }>('/v1/admin/archive/jobs?limit=50')
      setJobs(data.jobs || [])
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load jobs')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    if (getAdminArchiveToken()) {
      void refresh()
    }
  }, [refresh])

  const retryFailed = useCallback(async (jobID: string) => {
    await adminFetch(`/v1/admin/archive/jobs/${encodeURIComponent(jobID)}/retry-failed`, { method: 'POST' })
    await refresh()
  }, [refresh])

  const resumeJob = useCallback(async (jobID: string) => {
    await adminFetch(`/v1/admin/archive/jobs/${encodeURIComponent(jobID)}/resume`, { method: 'POST' })
    await refresh()
  }, [refresh])

  const cancelJob = useCallback(async (jobID: string) => {
    await adminFetch(`/v1/admin/archive/jobs/${encodeURIComponent(jobID)}/cancel`, { method: 'POST' })
    await refresh()
  }, [refresh])

  return { jobs, loading, error, refresh, retryFailed, resumeJob, cancelJob }
}

export function useArchiveCoverage() {
  const [report, setReport] = useState<CoverageReport | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const data = await adminFetch<CoverageReport>('/v1/admin/archive/coverage/summary')
      setReport(data)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load coverage')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    if (getAdminArchiveToken()) {
      void refresh()
    }
  }, [refresh])

  return { report, loading, error, refresh }
}
