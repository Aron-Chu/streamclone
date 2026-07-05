import type { AnalyticsStream } from '../apiTypes.ts'
import { getLocalDateString } from './consoleFormat.ts'

export interface HistoryStreamItem {
  id?: string
  streamId?: string
  startedAt?: string
}

export function resolveMatchedStream(
  streamIdParam: string,
  combinedStreams: AnalyticsStream[],
  routableStreams?: AnalyticsStream[],
): AnalyticsStream | undefined {
  if (!streamIdParam) return undefined

  const exactMatch = combinedStreams.find((s) => s.streamId === streamIdParam)
  if (exactMatch) return exactMatch

  if (/^\d{4}-\d{2}-\d{2}$/.test(streamIdParam)) {
    const pool = routableStreams ?? combinedStreams
    return pool.find((s) => {
      if (!s.startedAt) return false
      const date = new Date(s.startedAt)
      if (Number.isNaN(date.getTime())) return false

      const utcDateStr = date.toISOString().slice(0, 10)
      if (utcDateStr === streamIdParam) return true

      const year = date.getFullYear()
      const month = String(date.getMonth() + 1).padStart(2, '0')
      const day = String(date.getDate()).padStart(2, '0')
      const localDateStr = `${year}-${month}-${day}`
      return localDateStr === streamIdParam
    })
  }

  return undefined
}

export function resolveTargetQueryStreamId(
  streamIdParam: string,
  matchedStream: AnalyticsStream | undefined,
  historyItems: HistoryStreamItem[] | undefined,
  listsLoading: boolean,
): string | undefined {
  if (!streamIdParam) return ''
  if (/^\d+$/.test(streamIdParam)) return streamIdParam
  if (matchedStream) return matchedStream.streamId

  if (/^\d{4}-\d{2}-\d{2}$/.test(streamIdParam)) {
    if (historyItems?.length) {
      const fromHistory = historyItems.find((s) => {
        if (!s.startedAt) return false
        if (s.startedAt.slice(0, 10) === streamIdParam) return true
        return getLocalDateString(s.startedAt) === streamIdParam
      })
      const id = fromHistory?.streamId ?? fromHistory?.id
      if (id) return id
    }
    if (listsLoading) return undefined
    return undefined
  }

  return undefined
}

export function isDateSlugUnresolved(
  streamIdParam: string,
  matchedStream: AnalyticsStream | undefined,
  listsLoading: boolean,
): boolean {
  if (!streamIdParam || !/^\d{4}-\d{2}-\d{2}$/.test(streamIdParam)) return false
  if (listsLoading) return false
  return !matchedStream
}
