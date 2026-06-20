const STORAGE_KEY = 'pulseWireAnalystMode'

export function readAnalystMode(searchParams: URLSearchParams): boolean {
  const param = searchParams.get('analyst')
  if (param === '1') {
    try { sessionStorage.setItem(STORAGE_KEY, '1') } catch { /* ignore */ }
    return true
  }
  if (param === '0') {
    try { sessionStorage.removeItem(STORAGE_KEY) } catch { /* ignore */ }
    return false
  }
  try {
    return sessionStorage.getItem(STORAGE_KEY) === '1'
  } catch {
    return false
  }
}

export function writeAnalystMode(enabled: boolean): void {
  try {
    if (enabled) sessionStorage.setItem(STORAGE_KEY, '1')
    else sessionStorage.removeItem(STORAGE_KEY)
  } catch { /* ignore */ }
}
