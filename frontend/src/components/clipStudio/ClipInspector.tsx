import { useMemo, useState } from 'react'
import { CAPTION_EMOJIS, CaptionRichText } from '../../captionTokens'
import type {
  CaptionEffect,
  CaptionPreset,
  CaptionPosition,
  CaptionSize,
  CaptionWord,
  ClipperJob,
  ClipperTemplate,
} from '../../api'
import type { FormatPreset, InspectorTab } from './types'
import type { ChannelEmote } from '../../api'
import { EmoteScrollPicker } from './EmoteScrollPicker'
import { hookStrengthScore, predictedReachLabel } from './utils'

interface ClipInspectorProps {
  activeTab: InspectorTab
  onTabChange: (tab: InspectorTab) => void
  templates: ClipperTemplate[]
  selectedTemplateId: string | null
  formatPreset: FormatPreset
  captionPreset: CaptionPreset
  captionSize: CaptionSize
  captionPosition: CaptionPosition
  layout: string
  layoutSplitRatio: number
  captions: CaptionWord[]
  activeCaption: CaptionWord | null
  selectedCaptionIndex: number | null
  addTextMode: boolean
  channelEmotes: ChannelEmote[]
  job: ClipperJob
  trimStart: number
  trimEnd: number
  duration: number
  sourceUnavailable: boolean
  isTranscribing: boolean
  showEmojiPicker: boolean
  emojiTargetIndex: number | null
  onApplyTemplate: (template: ClipperTemplate) => void
  onFormatPresetChange: (preset: FormatPreset) => void
  onCaptionPresetChange: (preset: CaptionPreset) => void
  onCaptionSizeChange: (size: CaptionSize) => void
  onCaptionPositionChange: (pos: CaptionPosition) => void
  onLayoutChange: (layout: string) => void
  onLayoutSplitRatioChange: (ratio: number) => void
  onRetranscribe: () => void
  onSaveCaptions: () => void
  onAddCaptionRow: () => void
  onCaptionTextChange: (index: number, text: string) => void
  onCaptionTimeChange: (index: number, field: 'start' | 'end', val: string) => void
  onRemoveCaptionRow: (index: number) => void
  onSeekToCaption: (time: number) => void
  onSelectCaption: (index: number | null) => void
  onCaptionEffectChange: (index: number, effect: CaptionEffect) => void
  onResetCaptionPosition: (index: number) => void
  onAddTextModeChange: (enabled: boolean) => void
  onEmojiPickerToggle: (index: number) => void
  onInsertEmoji: (index: number, emoji: string) => void
  onInsertEmote: (index: number, emoteName: string) => void
  onCopyUploadPackage: () => void
}

const TABS: { id: InspectorTab; label: string }[] = [
  { id: 'template', label: 'Template' },
  { id: 'layout', label: 'Layout' },
  { id: 'captions', label: 'Captions' },
  { id: 'export', label: 'Export' },
]

const TEMPLATE_PREVIEW_COLORS: Record<string, string> = {
  tiktok_pop: 'linear-gradient(135deg,#fbbf24,#ef4444)',
  gaming_punch: 'linear-gradient(135deg,#22d3ee,#6366f1)',
  cinematic: 'linear-gradient(135deg,#1e293b,#475569)',
  clean_vertical: 'linear-gradient(135deg,#a78bfa,#ec4899)',
  subtitle_bar: 'linear-gradient(135deg,#0f172a,#334155)',
  stacked_reaction: 'linear-gradient(135deg,#f97316,#8b5cf6)',
  karaoke_highlight: 'linear-gradient(135deg,#fde047,#a855f7)',
  react_cam: 'linear-gradient(135deg,#06b6d4,#f43f5e)',
  minimal_subs: 'linear-gradient(135deg,#18181b,#3f3f46)',
  hype_moment: 'linear-gradient(135deg,#ef4444,#eab308)',
  podcast_clip: 'linear-gradient(135deg,#334155,#94a3b8)',
  streamer_rant: 'linear-gradient(135deg,#dc2626,#7c2d12)',
  hype_zoom: 'linear-gradient(135deg,#fde047,#f97316)',
  meme_impact: 'linear-gradient(135deg,#4ade80,#166534)',
  podcast_clean: 'linear-gradient(135deg,#94a3b8,#1e293b)',
  horror_vignette: 'linear-gradient(135deg,#450a0a,#18181b)',
  sports_energy: 'linear-gradient(135deg,#38bdf8,#dc2626)',
  minimal_white: 'linear-gradient(135deg,#f4f4f5,#d4d4d8)',
  vtuber_pastel: 'linear-gradient(135deg,#fbcfe8,#c4b5fd)',
  rage_clip: 'linear-gradient(135deg,#ef4444,#7f1d1d)',
  news_lower_third: 'linear-gradient(135deg,#1d4ed8,#0f172a)',
  slowmo_cinematic: 'linear-gradient(135deg,#312e81,#64748b)',
}

const CAPTION_EFFECTS: { id: CaptionEffect; label: string }[] = [
  { id: 'none', label: 'None' },
  { id: 'pop', label: 'Pop' },
  { id: 'glow', label: 'Glow' },
  { id: 'bounce', label: 'Bounce' },
  { id: 'shake', label: 'Shake' },
]

export function ClipInspector(props: ClipInspectorProps) {
  const hookScore = hookStrengthScore(props.job.moment_context)
  const reachLabel = predictedReachLabel(hookScore)
  const [templateSearch, setTemplateSearch] = useState('')
  const [templateFormatFilter, setTemplateFormatFilter] = useState<'all' | FormatPreset>('all')

  const filteredTemplates = useMemo(() => {
    const query = templateSearch.trim().toLowerCase()
    return props.templates.filter(t => {
      const matchesSearch = !query
        || t.name.toLowerCase().includes(query)
        || t.id.toLowerCase().includes(query)
        || t.description.toLowerCase().includes(query)
        || t.caption_preset.toLowerCase().includes(query)
      const matchesFormat = templateFormatFilter === 'all' || t.format_preset === templateFormatFilter
      return matchesSearch && matchesFormat
    })
  }, [props.templates, templateSearch, templateFormatFilter])

  const selectedCaption = props.selectedCaptionIndex !== null
    ? props.captions[props.selectedCaptionIndex] ?? null
    : null

  const emoteInsertIndex = props.emojiTargetIndex ?? props.selectedCaptionIndex

  return (
    <aside className="clip-inspector">
      <div className="clip-inspector-tabs">
        {TABS.map(tab => (
          <button
            key={tab.id}
            type="button"
            className={`clip-inspector-tab ${props.activeTab === tab.id ? 'active' : ''}`}
            onClick={() => props.onTabChange(tab.id)}
          >
            {tab.label}
          </button>
        ))}
      </div>

      <div className="clip-inspector-body">
        {props.activeTab === 'template' && (
          <>
            <div className="clip-studio-section-title">Visual templates</div>
            <div className="template-filter-bar">
              <input
                type="search"
                className="template-search-input"
                placeholder="Search templates..."
                value={templateSearch}
                onChange={e => setTemplateSearch(e.target.value)}
              />
              <select
                className="template-filter-select"
                value={templateFormatFilter}
                onChange={e => setTemplateFormatFilter(e.target.value as 'all' | FormatPreset)}
              >
                <option value="all">All formats</option>
                <option value="tiktok">TikTok</option>
                <option value="youtube_short">YT Shorts</option>
                <option value="instagram_reel">IG Reel</option>
                <option value="youtube">YouTube</option>
                <option value="twitter">X</option>
              </select>
            </div>
            {filteredTemplates.length === 0 ? (
              <p className="clip-studio-caption-hint">No templates match your filters.</p>
            ) : (
            <div className="template-tile-grid">
              {filteredTemplates.map(t => (
                <button
                  key={t.id}
                  type="button"
                  className={`template-tile ${props.selectedTemplateId === t.id ? 'active' : ''}`}
                  onClick={() => props.onApplyTemplate(t)}
                  title={t.description}
                >
                  <span
                    className="template-tile-preview"
                    style={{ background: TEMPLATE_PREVIEW_COLORS[t.id] || 'linear-gradient(135deg,#27272a,#52525b)' }}
                  />
                  <span className="template-tile-name">{t.name}</span>
                  <span className="template-tile-meta">{t.caption_preset}</span>
                </button>
              ))}
            </div>
            )}
          </>
        )}

        {props.activeTab === 'layout' && (
          <>
            <div className="clip-studio-section-title">Video layout</div>
            <div className="clip-studio-presets-grid">
              {([
                ['blur_bg_center', 'Blur + center'],
                ['stacked_game_face', 'Stacked face/game'],
              ] as const).map(([id, label]) => (
                <div
                  key={id}
                  className={`preset-card ${props.layout === id ? 'active' : ''}`}
                  onClick={() => props.onLayoutChange(id)}
                >
                  <span>{label}</span>
                </div>
              ))}
            </div>
            {props.layout === 'stacked_game_face' && (
              <div className="layout-split-control">
                <label>
                  Facecam ratio: {(props.layoutSplitRatio * 100).toFixed(0)}%
                  <input
                    type="range"
                    min={0.2}
                    max={0.55}
                    step={0.01}
                    value={props.layoutSplitRatio}
                    onChange={e => props.onLayoutSplitRatioChange(parseFloat(e.target.value))}
                  />
                </label>
              </div>
            )}
          </>
        )}

        {props.activeTab === 'captions' && (
          <>
            <div className="clip-studio-section-title">Caption style</div>
            <div className="clip-studio-presets-grid clip-studio-presets-grid-3">
              {([
                ['default', 'Default'],
                ['karaoke_pop', 'Karaoke'],
                ['tiktok_pop', 'TikTok Pop'],
                ['subtitle_bar', 'Sub Bar'],
                ['gaming', 'Gaming'],
                ['none', 'None'],
              ] as const).map(([id, label]) => (
                <div
                  key={id}
                  className={`preset-card ${props.captionPreset === id ? 'active' : ''}`}
                  onClick={() => props.onCaptionPresetChange(id)}
                >
                  <span>{label}</span>
                </div>
              ))}
            </div>
            <div className="clip-studio-caption-customize">
              <div className="clip-studio-caption-customize-row">
                <span className="clip-studio-caption-customize-label">Size</span>
                <div className="clip-studio-presets-grid clip-studio-presets-grid-3">
                  {(['sm', 'md', 'lg'] as const).map(id => (
                    <div
                      key={id}
                      className={`preset-card ${props.captionSize === id ? 'active' : ''}`}
                      onClick={() => props.onCaptionSizeChange(id)}
                    >
                      <span>{id === 'sm' ? 'Small' : id === 'md' ? 'Medium' : 'Large'}</span>
                    </div>
                  ))}
                </div>
              </div>
              <div className="clip-studio-caption-customize-row">
                <span className="clip-studio-caption-customize-label">Position</span>
                <div className="clip-studio-presets-grid clip-studio-presets-grid-3">
                  {(['top', 'center', 'bottom'] as const).map(id => (
                    <div
                      key={id}
                      className={`preset-card ${props.captionPosition === id ? 'active' : ''}`}
                      onClick={() => props.onCaptionPositionChange(id)}
                    >
                      <span>{id}</span>
                    </div>
                  ))}
                </div>
              </div>
              <div className="clip-studio-caption-customize-row">
                <span className="clip-studio-caption-customize-label">Canvas placement</span>
                <button
                  type="button"
                  className={`preset-card canvas-place-toggle ${props.addTextMode ? 'active' : ''}`}
                  onClick={() => props.onAddTextModeChange(!props.addTextMode)}
                >
                  <span>{props.addTextMode ? 'Placing on canvas' : 'Place on canvas'}</span>
                </button>
                <p className="clip-studio-caption-hint">
                  Click the video to add or reposition captions. ASR lines without placement use the global position above.
                </p>
              </div>
              {selectedCaption && props.selectedCaptionIndex !== null && (
                <div className="clip-studio-caption-customize-row">
                  <span className="clip-studio-caption-customize-label">Selected caption effect</span>
                  <div className="clip-studio-presets-grid clip-studio-presets-grid-3">
                    {CAPTION_EFFECTS.map(({ id, label }) => (
                      <button
                        key={id}
                        type="button"
                        className={`preset-card ${(selectedCaption.effect ?? 'none') === id ? 'active' : ''}`}
                        onClick={() => props.onCaptionEffectChange(props.selectedCaptionIndex!, id)}
                      >
                        <span>{label}</span>
                      </button>
                    ))}
                  </div>
                  {selectedCaption.transform && (
                    <button
                      type="button"
                      className="clip-studio-btn-ghost caption-reset-position"
                      onClick={() => props.onResetCaptionPosition(props.selectedCaptionIndex!)}
                    >
                      Reset position
                    </button>
                  )}
                </div>
              )}
            </div>

            <div className="clip-studio-section-header">
              <div className="clip-studio-section-title">Caption lines</div>
              <div className="clip-studio-section-actions">
                <button className="clip-studio-btn-ghost" onClick={props.onRetranscribe} disabled={props.sourceUnavailable || props.isTranscribing}>
                  {props.isTranscribing ? 'Transcribing...' : 'Re-transcribe'}
                </button>
                <button className="clip-studio-btn-ghost" onClick={props.onSaveCaptions}>Save</button>
              </div>
            </div>

            <EmoteScrollPicker
              emotes={props.channelEmotes}
              onPick={name => {
                if (emoteInsertIndex === null) return
                props.onInsertEmote(emoteInsertIndex, name)
              }}
            />

            {props.showEmojiPicker && props.emojiTargetIndex !== null && (
              <div className="clip-studio-emoji-picker">
                {CAPTION_EMOJIS.map(emoji => (
                  <button
                    key={emoji}
                    type="button"
                    className="clip-studio-emoji-btn"
                    onClick={() => props.onInsertEmoji(props.emojiTargetIndex!, emoji)}
                  >
                    {emoji}
                  </button>
                ))}
              </div>
            )}

            <div className="clip-studio-caption-list">
              {props.captions.map((cap, idx) => (
                <div
                  key={idx}
                  className={`caption-row ${props.activeCaption === cap ? 'active' : ''}${props.selectedCaptionIndex === idx ? ' caption-row-selected' : ''}`}
                  onClick={() => props.onSelectCaption(idx)}
                >
                  <div className="caption-row-times" onClick={e => e.stopPropagation()}>
                    <input type="number" step="0.1" className="caption-time-input" value={cap.start}
                      onChange={e => props.onCaptionTimeChange(idx, 'start', e.target.value)} />
                    <input type="number" step="0.1" className="caption-time-input" value={cap.end}
                      onChange={e => props.onCaptionTimeChange(idx, 'end', e.target.value)} />
                  </div>
                  <input type="text" className="caption-text-input" value={cap.text}
                    onChange={e => props.onCaptionTextChange(idx, e.target.value)}
                    onClick={e => { e.stopPropagation(); props.onSeekToCaption(cap.start) }} />
                  <button type="button" className="btn-caption-emoji" title="Insert emote or emoji"
                    onClick={e => { e.stopPropagation(); props.onEmojiPickerToggle(idx) }}>😀</button>
                  <button className="btn-remove-caption" onClick={e => { e.stopPropagation(); props.onRemoveCaptionRow(idx) }} aria-label="Remove caption">&times;</button>
                </div>
              ))}
            </div>
            <button className="btn-add-caption" onClick={props.onAddCaptionRow}>+ Add caption</button>
          </>
        )}

        {props.activeTab === 'export' && (
          <>
            {reachLabel && (
              <div className={`predicted-reach-badge predicted-reach-${reachLabel.toLowerCase()}`}>
                <span className="predicted-reach-label">Predicted reach</span>
                <span className="predicted-reach-value">{reachLabel}</span>
              </div>
            )}

            <div className="clip-studio-section-title">Output format</div>
            <div className="clip-studio-presets-grid">
              {([
                ['tiktok', 'TikTok 9:16'],
                ['youtube_short', 'YT Shorts'],
                ['instagram_reel', 'IG Reel'],
                ['youtube', 'YouTube 16:9'],
                ['twitter', 'X 1:1'],
              ] as const).map(([id, label]) => (
                <div
                  key={id}
                  className={`preset-card ${props.formatPreset === id ? 'active' : ''}`}
                  onClick={() => props.onFormatPresetChange(id)}
                >
                  <span>{label}</span>
                </div>
              ))}
            </div>

            <div className="clip-studio-section-title">Upload package</div>
            <p className="clip-studio-caption-hint">
              Copy title, hashtags, Twitch clip URL, and moment stats for TikTok/Shorts upload.
            </p>
            <button type="button" className="btn-copy-package" onClick={props.onCopyUploadPackage}>
              Copy upload package
            </button>

            {props.activeCaption && (
              <div className="export-caption-preview">
                <span className="clip-studio-caption-customize-label">Live caption preview</span>
                <CaptionRichText text={props.activeCaption.text} emotes={props.channelEmotes} />
              </div>
            )}
          </>
        )}
      </div>
    </aside>
  )
}
