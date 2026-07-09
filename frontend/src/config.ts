type RuntimeConfig = {
  metadataUrl?: string
  videoUrl?: string
  emoteUrl?: string
  analyticsUrl?: string
  chatWs?: string
  chatHttp?: string
  clipperUrl?: string
  replayforgeUiUrl?: string
  maxRetainedMessages?: string | number
  streamcloneProfile?: string
  setupControlToken?: string
  setupControlUrl?: string
  setupControlWakeEnabled?: string | boolean
  devTokenImportEnabled?: string | boolean
  pulseWireEnabled?: string | boolean
  hlsLowLatencyEnabled?: string | boolean
  adaptiveLiveLatencyEnabled?: string | boolean
  hlsCdnBearer?: string
  installId?: string
}

declare global {
  interface Window {
    __STREAMCLONE_CONFIG__?: RuntimeConfig
  }
}

const runtime = typeof window === 'undefined' ? {} : window.__STREAMCLONE_CONFIG__ ?? {}
const viteEnv = (typeof import.meta !== 'undefined' && import.meta.env)
  ? import.meta.env as Record<string, string | undefined>
  : {}
const browserOrigin = typeof window === 'undefined' ? '' : window.location.origin
const browserWsOrigin = typeof window === 'undefined'
  ? ''
  : `${window.location.protocol === 'https:' ? 'wss' : 'ws'}://${window.location.host}`

function resolveHttp(value: string | undefined, fallback: string) {
  if (value === 'auto') return browserOrigin
  return value || fallback
}

function resolveWs(value: string | undefined, fallback: string) {
  if (value === 'auto') return `${browserWsOrigin}/v1/ws`
  return value || fallback
}

export function normalizeBrowserOriginUrl(value: string | undefined, pathPrefixes: string[]) {
  if (!value || typeof window === 'undefined') return value || ''
  try {
    const url = new URL(value, window.location.origin)
    if (!pathPrefixes.some(prefix => url.pathname.startsWith(prefix))) {
      return url.toString()
    }
    const hitsRawMediaMtx = url.port === '8888' || url.hostname === 'mediamtx'
    if (url.origin === window.location.origin && !hitsRawMediaMtx) {
      return url.toString()
    }
    url.protocol = window.location.protocol
    url.host = window.location.host
    return url.toString()
  } catch {
    return value
  }
}

function resolveHlsCdnBearer() {
  const configured = String(runtime.hlsCdnBearer ?? viteEnv.VITE_HLS_CDN_BEARER ?? '').trim()
  if (configured) return configured
  if (typeof window !== 'undefined') {
    const { hostname, port } = window.location
    if ((hostname === 'localhost' || hostname === '127.0.0.1') && port === '8090') {
      return 'streamclone-local-hls-cdn'
    }
  }
  return ''
}

export const HLS_CDN_BEARER = resolveHlsCdnBearer()
export const METADATA = resolveHttp(runtime.metadataUrl || viteEnv.VITE_METADATA_URL, browserOrigin || 'http://localhost:8081')
export const VIDEO = resolveHttp(runtime.videoUrl || viteEnv.VITE_VIDEO_URL, browserOrigin || 'http://localhost:8082')
export const EMOTE = resolveHttp(runtime.emoteUrl || viteEnv.VITE_EMOTE_URL, browserOrigin || 'http://localhost:8084')
export const ANALYTICS = resolveHttp(runtime.analyticsUrl || viteEnv.VITE_ANALYTICS_URL, browserOrigin || 'http://localhost:8086')
export const CHAT_WS = resolveWs(runtime.chatWs || viteEnv.VITE_CHAT_WS, browserWsOrigin ? `${browserWsOrigin}/v1/ws` : 'ws://localhost:8083/v1/ws')
export const CHAT_HTTP = resolveHttp(runtime.chatHttp || viteEnv.VITE_CHAT_HTTP, browserOrigin || 'http://localhost:8083')
export const CLIPPER = resolveHttp(runtime.clipperUrl || viteEnv.VITE_CLIPPER_URL, 'http://localhost:8095')
// RF-P5-013: the clipper mutation Auth_Token is injected server-side by the
// same-origin /v1/clipper/* proxy (Caddy header_up from CLIPPER_WEBHOOK_TOKEN).
// It is deliberately never read into the browser bundle/config.
export const REPLAYFORGE_UI = resolveHttp(
  runtime.replayforgeUiUrl || viteEnv.VITE_REPLAYFORGE_UI_URL,
  'http://localhost:8096',
)
export const MAX_RETAINED_MESSAGES = Number(runtime.maxRetainedMessages ?? viteEnv.VITE_MAX_RETAINED_MESSAGES ?? 250)
export const STREAMCLONE_PROFILE = String(runtime.streamcloneProfile ?? viteEnv.VITE_STREAMCLONE_PROFILE ?? 'core').toLowerCase()
export const DEV_TOKEN_IMPORT_ENABLED = String(runtime.devTokenImportEnabled ?? viteEnv.VITE_TWITCH_DEV_TOKEN_IMPORT_ENABLED ?? 'false') === 'true'
export const SETUP_CONTROL_TOKEN = String(runtime.setupControlToken ?? viteEnv.VITE_SETUP_CONTROL_TOKEN ?? '')
export const SETUP_CONTROL_WAKE_ENABLED = ['true', '1'].includes(
  String(runtime.setupControlWakeEnabled ?? viteEnv.VITE_SETUP_CONTROL_WAKE_ENABLED ?? 'false').toLowerCase(),
)
export const SETUP_CONTROL_AVAILABLE = Boolean(SETUP_CONTROL_TOKEN)
  && (
    !viteEnv.DEV
    || browserOrigin.endsWith(':8090')
    || String(viteEnv.VITE_SETUP_CONTROL_ENABLE ?? 'false') === 'true'
  )

function resolveSetupControlBase() {
  const configured = String(runtime.setupControlUrl ?? viteEnv.VITE_SETUP_CONTROL_URL ?? '').trim()
  if (configured) return configured.replace(/\/$/, '')
  if (typeof window !== 'undefined') {
    const { hostname, port } = window.location
    if ((hostname === 'localhost' || hostname === '127.0.0.1') && port === '8090') {
      return 'http://127.0.0.1:9191'
    }
  }
  return browserOrigin ? `${browserOrigin}/v1/setup-control` : 'http://127.0.0.1:9191'
}

export const SETUP_CONTROL_BASE = resolveSetupControlBase()
export const STREAMCLONE_INSTALL_ID = String(runtime.installId ?? viteEnv.VITE_STREAMCLONE_INSTALL_ID ?? '').trim()
export const PULSE_WIRE_ENABLED = ['true', '1'].includes(
  String(runtime.pulseWireEnabled ?? viteEnv.VITE_PULSE_WIRE_ENABLED ?? 'false').toLowerCase(),
)
export const HLS_LOW_LATENCY_ENABLED = ['true', '1'].includes(
  String(runtime.hlsLowLatencyEnabled ?? viteEnv.VITE_HLS_LOW_LATENCY_ENABLED ?? 'false').toLowerCase(),
)
export const ADAPTIVE_LIVE_LATENCY_ENABLED = ['true', '1'].includes(
  String(runtime.adaptiveLiveLatencyEnabled ?? viteEnv.VITE_ADAPTIVE_LIVE_LATENCY_ENABLED ?? 'false').toLowerCase(),
)

export const ADMIN_ARCHIVE_UI_ENABLED = ['true', '1'].includes(
  String(viteEnv.VITE_ADMIN_ARCHIVE_UI_ENABLED ?? 'false').toLowerCase(),
)

/** TASK-021B: token UI only on localhost or HTTPS — public BearHost HTTP shows CLI instructions. */
export const ADMIN_ARCHIVE_UI_ALLOWED = (() => {
  if (ADMIN_ARCHIVE_UI_ENABLED) return true
  if (typeof window === 'undefined') return false
  const { protocol, hostname } = window.location
  if (protocol === 'https:') return true
  return hostname === 'localhost' || hostname === '127.0.0.1'
})()
