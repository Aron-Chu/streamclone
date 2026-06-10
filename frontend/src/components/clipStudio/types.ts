import type {
  CaptionPreset,
  CaptionPosition,
  CaptionSize,
  CaptionWord,
  ChannelEmote,
  ClipperJob,
  ClipperTemplate,
} from '../../api'

export type PreviewMode = 'source' | 'final'
export type FormatPreset = 'tiktok' | 'youtube' | 'youtube_short' | 'instagram_reel' | 'twitter'
export type RenderStatus = 'idle' | 'rendering' | 'success' | 'failed'
export type InspectorTab = 'template' | 'layout' | 'captions' | 'export'

export interface ClipStudioState {
  job: ClipperJob
  captions: CaptionWord[]
  templates: ClipperTemplate[]
  selectedTemplateId: string | null
  formatPreset: FormatPreset
  captionPreset: CaptionPreset
  captionSize: CaptionSize
  captionPosition: CaptionPosition
  layout: string
  layoutSplitRatio: number
  previewMode: PreviewMode
  trimStart: number
  trimEnd: number
  duration: number
  currentTime: number
  isPlaying: boolean
  renderStatus: RenderStatus
  channelEmotes: ChannelEmote[]
}

export { type CaptionWord, type ClipperJob, type ClipperTemplate, type ChannelEmote }
