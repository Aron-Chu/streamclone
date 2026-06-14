export type ReplayState =
  | 'loading'
  | 'error'
  | 'unavailable'
  | 'empty_minute'
  | 'has_messages'
  | 'syncing_with_messages'
  | 'syncing_empty'

export function computeBucketStart(offsetSeconds: number): number {
  return Math.floor(offsetSeconds / 60) * 60
}

export function computeBucketEnd(bucketStart: number): number {
  return bucketStart + 59
}

export interface ReplayData {
  messages: unknown[]
  unavailable: boolean
}

export function classifyReplayState(
  data: ReplayData | undefined,
  isLoading: boolean,
  isError: boolean,
  isSyncing: boolean,
): ReplayState {
  if (isLoading) return 'loading'
  if (isError) return 'error'
  if (data?.unavailable) return 'unavailable'
  if (!data) return 'loading'

  const hasMessages = data.messages.length > 0

  if (isSyncing && hasMessages) return 'syncing_with_messages'
  if (isSyncing && !hasMessages) return 'syncing_empty'
  if (hasMessages) return 'has_messages'
  return 'empty_minute'
}
