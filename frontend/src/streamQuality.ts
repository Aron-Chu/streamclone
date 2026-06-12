// Shared quality mapping so prewarm requests the exact quality string the
// channel page will use — a mismatch forces a relay restart instead of a join.
export const autoHighStableQuality = 'auto-high-stable'
export const autoHighStableChain = '720p60,720p,1080p60,1080p,best'
export const defaultQualityOptions = [autoHighStableQuality, 'best', '1080p60', '1080p', '720p60', '720p', '480p', '360p']

export function requestQuality(value: string) {
  return value === autoHighStableQuality ? autoHighStableChain : value
}
