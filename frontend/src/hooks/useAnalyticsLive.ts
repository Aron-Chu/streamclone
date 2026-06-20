import { useQuery } from '@tanstack/react-query'

import { getAnalyticsLive, type AnalyticsStreamDetail } from '../api.ts'
import { LIVE_REQUEST_TIMEOUT_MS } from '../utils/liveStats.ts'

export function analyticsLiveQueryKey(login: string) {
  return ['analytics-live', login] as const
}

export type UseAnalyticsLiveOptions = {
  enabled?: boolean
  refetchInterval?: number | false
  /** Reject if getAnalyticsLive does not resolve within LIVE_REQUEST_TIMEOUT_MS. */
  withTimeout?: boolean
}

function fetchLiveWithTimeout(login: string): Promise<AnalyticsStreamDetail> {
  return new Promise<AnalyticsStreamDetail>((resolve, reject) => {
    const timer = setTimeout(
      () => reject(new Error('live stats request timed out')),
      LIVE_REQUEST_TIMEOUT_MS,
    )
    getAnalyticsLive(login).then(
      value => {
        clearTimeout(timer)
        resolve(value)
      },
      err => {
        clearTimeout(timer)
        reject(err)
      },
    )
  })
}

export function useAnalyticsLive(login: string, options: UseAnalyticsLiveOptions = {}) {
  const { enabled = true, refetchInterval, withTimeout = false } = options
  return useQuery({
    queryKey: analyticsLiveQueryKey(login),
    queryFn: () => (withTimeout ? fetchLiveWithTimeout(login) : getAnalyticsLive(login)),
    enabled: Boolean(login) && enabled,
    staleTime: 15_000,
    retry: false,
    refetchInterval: refetchInterval ?? (query =>
      query.state.data?.state === 'live' ? 15_000 : 60_000),
  })
}
