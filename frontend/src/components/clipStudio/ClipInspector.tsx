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
  { id: 'layout', label: 'Layout' },
  { id: 'captions', label: 'Captions' },
  { id: 'export', label: 'Export' },
]

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

  const selectedCaption = props.selectedCaptionIndex !== null
    ? props.captions[props.selectedCaptionIndex] ?? null
    : null

  const emoteInsertIndex = props.emojiTargetIndex ?? props.selectedCaptionIndex

  return (
    <aside className="clip-inspector flex min-h-0 flex-1 flex-col overflow-hidden">
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
