type RuntimeConfig = {
  metadataUrl?: string
  videoUrl?: string
  emoteUrl?: string
  analyticsUrl?: string
  chatWs?: string
  chatHttp?: string
  clipperUrl?: string
  clipperToken?: string
  maxRetainedMessages?: string | number
  streamcloneProfile?: string
  setupControlToken?: string
  devTokenImportEnabled?: string | boolean
  installId?: string
}

declare global {
  interface Window {
    __STREAMCLONE_CONFIG__?: RuntimeConfig
  }
}

const runtime = typeof window === 'undefined' ? {} : window.__STREAMCLONE_CONFIG__ ?? {}
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
    if (url.origin === window.location.origin) {
      return url.toString()
    }
    url.protocol = window.location.protocol
    url.host = window.location.host
    return url.toString()
  } catch {
    return value
  }
}

export const METADATA = resolveHttp(runtime.metadataUrl || (import.meta.env.VITE_METADATA_URL as string), browserOrigin || 'http://localhost:8081')
export const VIDEO = resolveHttp(runtime.videoUrl || (import.meta.env.VITE_VIDEO_URL as string), browserOrigin || 'http://localhost:8082')
export const EMOTE = resolveHttp(runtime.emoteUrl || (import.meta.env.VITE_EMOTE_URL as string), browserOrigin || 'http://localhost:8084')
export const ANALYTICS = resolveHttp(runtime.analyticsUrl || (import.meta.env.VITE_ANALYTICS_URL as string), browserOrigin || 'http://localhost:8086')
export const CHAT_WS = resolveWs(runtime.chatWs || (import.meta.env.VITE_CHAT_WS as string), browserWsOrigin ? `${browserWsOrigin}/v1/ws` : 'ws://localhost:8083/v1/ws')
export const CHAT_HTTP = resolveHttp(runtime.chatHttp || (import.meta.env.VITE_CHAT_HTTP as string), browserOrigin || 'http://localhost:8083')
export const CLIPPER = resolveHttp(runtime.clipperUrl || (import.meta.env.VITE_CLIPPER_URL as string), 'http://localhost:8095')
export const CLIPPER_TOKEN = String(runtime.clipperToken ?? import.meta.env.VITE_CLIPPER_TOKEN ?? '')
export const MAX_RETAINED_MESSAGES = Number(runtime.maxRetainedMessages ?? import.meta.env.VITE_MAX_RETAINED_MESSAGES ?? 250)
export const STREAMCLONE_PROFILE = String(runtime.streamcloneProfile ?? import.meta.env.VITE_STREAMCLONE_PROFILE ?? 'core').toLowerCase()
export const DEV_TOKEN_IMPORT_ENABLED = String(runtime.devTokenImportEnabled ?? import.meta.env.VITE_TWITCH_DEV_TOKEN_IMPORT_ENABLED ?? 'false') === 'true'
export const SETUP_CONTROL_TOKEN = String(runtime.setupControlToken ?? import.meta.env.VITE_SETUP_CONTROL_TOKEN ?? '')
export const STREAMCLONE_INSTALL_ID = String(runtime.installId ?? import.meta.env.VITE_STREAMCLONE_INSTALL_ID ?? '').trim()
