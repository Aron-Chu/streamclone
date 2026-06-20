/** Server-resolved display URL from Pulse Wire API. Use as-is; do not derive client-side. */
export function pulseWireDisplayThumbnail(displayThumbnailUrl?: string): string | undefined {
  const raw = (displayThumbnailUrl ?? '').trim()
  return raw || undefined
}
