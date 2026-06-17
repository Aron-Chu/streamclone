import { getSetupControlHealth } from './api'
import { SETUP_CONTROL_WAKE_ENABLED } from './config'

const WAKE_URL = 'streamclone://ensure-setup-control'
const POLL_MS = 500
const TIMEOUT_MS = 30_000

export async function wakeSetupControl(): Promise<boolean> {
  if (!SETUP_CONTROL_WAKE_ENABLED) return false

  try {
    const health = await getSetupControlHealth()
    if (health?.ok) return true
  } catch {
    // helper offline — try URL wake below
  }

  window.location.href = WAKE_URL

  const deadline = Date.now() + TIMEOUT_MS
  while (Date.now() < deadline) {
    await new Promise(resolve => window.setTimeout(resolve, POLL_MS))
    try {
      const health = await getSetupControlHealth()
      if (health?.ok) return true
    } catch {
      // keep polling until timeout
    }
  }
  return false
}
