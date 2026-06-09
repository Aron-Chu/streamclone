import { useEffect } from 'react'
import { create } from 'zustand'
import type { EmoteProvider } from './api'

export type ClipPeriod = '24h' | '7d' | '30d' | '365d' | 'all'
export type StatsPeriod = ClipPeriod
export type ThemeName = 'obsidian' | 'midnight' | 'daylight'
export type PlaybackLatencyMode = 'stable' | 'fast' | 'instant'
export type VideoFitMode = 'fit' | 'fill'
export type BottomDensityMode = 'comfortable' | 'dense'

export interface UiSettings {
  previewAutoplay: boolean
  previewMuted: boolean
  theme: ThemeName
  railSections: {
    live: boolean
    offline: boolean
    top: boolean
  }
  emoteProviders: EmoteProvider[]
  emoteAutoLoad: boolean
  preferredQuality: string
  playbackLatencyMode: PlaybackLatencyMode
  videoFit: VideoFitMode
  bottomDensity: BottomDensityMode
}

const storageKey = 'streamclone:ui-settings'

const defaults: UiSettings = {
  previewAutoplay: true,
  previewMuted: true,
  theme: 'obsidian',
  railSections: { live: true, offline: true, top: true },
  emoteProviders: ['seventv', 'twitch'],
  emoteAutoLoad: false,
  preferredQuality: 'auto-high-stable',
  playbackLatencyMode: 'fast',
  videoFit: 'fit',
  bottomDensity: 'comfortable',
}

function loadSettings(): UiSettings {
  if (typeof localStorage === 'undefined') return defaults
  try {
    const parsed = JSON.parse(localStorage.getItem(storageKey) || '{}') as Partial<UiSettings>
    const providers = (parsed.emoteProviders ?? defaults.emoteProviders)
      .filter((provider): provider is EmoteProvider => provider === 'seventv' || provider === 'twitch')
    const preferredQuality = typeof parsed.preferredQuality === 'string' && parsed.preferredQuality.trim() ? parsed.preferredQuality : defaults.preferredQuality
    const playbackLatencyMode: PlaybackLatencyMode =
      parsed.playbackLatencyMode === 'instant' || parsed.playbackLatencyMode === 'fast' || parsed.playbackLatencyMode === 'stable'
        ? parsed.playbackLatencyMode
        : defaults.playbackLatencyMode
    const videoFit = parsed.videoFit === 'fill' ? 'fill' : defaults.videoFit
    const bottomDensity = parsed.bottomDensity === 'dense' ? 'dense' : defaults.bottomDensity
    return {
      ...defaults,
      ...parsed,
      railSections: { ...defaults.railSections, ...parsed.railSections },
      emoteProviders: providers.length ? providers : defaults.emoteProviders,
      preferredQuality,
      playbackLatencyMode,
      videoFit,
      bottomDensity,
    }
  } catch {
    return defaults
  }
}

interface SettingsStore {
  settings: UiSettings
  updateSettings: (patch: SettingsPatch) => void
  toggleRailSection: (section: keyof UiSettings['railSections']) => void
  toggleEmoteProvider: (provider: EmoteProvider) => void
}

type SettingsPatch = Omit<Partial<UiSettings>, 'railSections'> & {
  railSections?: Partial<UiSettings['railSections']>
}

export const useUiSettings = create<SettingsStore>(set => ({
  settings: loadSettings(),
  updateSettings: patch => set(state => {
    const next = {
      ...state.settings,
      ...patch,
      railSections: patch.railSections ? { ...state.settings.railSections, ...patch.railSections } : state.settings.railSections,
    }
    persist(next)
    return { settings: next }
  }),
  toggleRailSection: section => set(state => {
    const next = {
      ...state.settings,
      railSections: {
        ...state.settings.railSections,
        [section]: !state.settings.railSections[section],
      },
    }
    persist(next)
    return { settings: next }
  }),
  toggleEmoteProvider: provider => set(state => {
    const current = state.settings.emoteProviders
    const providers = current.includes(provider)
      ? current.length === 1 ? current : current.filter(item => item !== provider)
      : [...current, provider]
    const next = { ...state.settings, emoteProviders: providers }
    persist(next)
    return { settings: next }
  }),
}))

function persist(settings: UiSettings) {
  try {
    localStorage.setItem(storageKey, JSON.stringify(settings))
  } catch {
    return
  }
}

export function useThemeEffect(theme: ThemeName) {
  useEffect(() => {
    document.documentElement.dataset.theme = theme
  }, [theme])
}
