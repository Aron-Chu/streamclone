const VOD_ID_PATTERN = /^\d{5,20}$/

export function isVodId(value: string): boolean {
  return VOD_ID_PATTERN.test(value)
}

export function normalizeVodId(raw: string | null | undefined): string | null {
  if (raw == null) {
    return null
  }
  const stripped = String(raw).replace(/\s+/g, '')
  if (stripped === '') {
    return null
  }
  if (/videos\//i.test(stripped)) {
    return null
  }
  return VOD_ID_PATTERN.test(stripped) ? stripped : null
}
