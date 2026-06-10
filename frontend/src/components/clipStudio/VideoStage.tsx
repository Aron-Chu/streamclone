import type { RefObject } from 'react'
import { CaptionRichText } from '../../captionTokens'
import type { CaptionPreset, CaptionPosition, CaptionSize, CaptionWord, ChannelEmote } from '../../api'
import type { FormatPreset, PreviewMode } from './types'
import { CaptionOverlayEditor } from './CaptionOverlayEditor'

interface VideoStageProps {
  videoRef: RefObject<HTMLVideoElement | null>
  videoSrc: string
  previewMode: PreviewMode
  formatPreset: FormatPreset
  canPreviewSource: boolean
  canPreviewFinal: boolean
  failureMessage: string
  jobState: string
  captionPreset: CaptionPreset
  captionSize: CaptionSize
  captionPosition: CaptionPosition
  captions: CaptionWord[]
  currentTime: number
  activeCaption: CaptionWord | null
  activeWordIndex: number
  channelEmotes: ChannelEmote[]
  selectedCaptionIndex: number | null
  addTextMode: boolean
  onPreviewModeChange: (mode: PreviewMode) => void
  onTimeUpdate: () => void
  onLoadedMetadata: () => void
  onTogglePlay: () => void
  onPlay: () => void
  onPause: () => void
  onSelectCaption: (index: number | null) => void
  onUpdateCaption: (index: number, patch: Partial<CaptionWord>) => void
  onAddCaptionAt: (x: number, y: number) => void
}

export function VideoStage({
  videoRef,
  videoSrc,
  previewMode,
  formatPreset,
  canPreviewSource,
  canPreviewFinal,
  failureMessage,
  jobState,
  captionPreset,
  captionSize,
  captionPosition,
  captions,
  currentTime,
  activeCaption,
  activeWordIndex,
  channelEmotes,
  selectedCaptionIndex,
  addTextMode,
  onPreviewModeChange,
  onTimeUpdate,
  onLoadedMetadata,
  onTogglePlay,
  onPlay,
  onPause,
  onSelectCaption,
  onUpdateCaption,
  onAddCaptionAt,
}: VideoStageProps) {
  const aspectClass = formatPreset === 'youtube' ? 'youtube' : formatPreset === 'twitter' ? 'twitter' : 'tiktok'
  const sourceUnavailable = !canPreviewSource
  const showCanvasEditor = previewMode === 'source' && captionPreset !== 'none'

  const renderGlobalCaptionOverlay = () => {
    if (!showCanvasEditor || !activeCaption || activeCaption.transform) return null
    const overlayClass = [
      'clip-studio-captions-overlay',
      `subtitle-overlay-${captionPreset}`,
      `caption-size-${captionSize}`,
      `caption-pos-${captionPosition}`,
    ].join(' ')
    const isKaraoke = captionPreset === 'karaoke_pop' || captionPreset === 'tiktok_pop'
    if (isKaraoke && activeCaption.words?.length) {
      return (
        <div className={overlayClass}>
          {activeCaption.words.map((w, i) => (
            <span key={i} className={i === activeWordIndex ? 'karaoke-word-active' : 'karaoke-word'}>
              <CaptionRichText text={`${w.text} `} emotes={channelEmotes} />
            </span>
          ))}
        </div>
      )
    }
    return (
      <div className={overlayClass}>
        <CaptionRichText text={activeCaption.text} emotes={channelEmotes} />
      </div>
    )
  }

  const handleVideoClick = () => {
    if (addTextMode) return
    onTogglePlay()
  }

  return (
    <section className="video-stage">
      <div className="clip-studio-preview-toolbar">
        <button
          className={`preview-toggle ${previewMode === 'source' ? 'active' : ''}`}
          onClick={() => onPreviewModeChange('source')}
          disabled={!canPreviewSource}
        >
          Source
        </button>
        <button
          className={`preview-toggle ${previewMode === 'final' ? 'active' : ''}`}
          onClick={() => onPreviewModeChange('final')}
          disabled={!canPreviewFinal}
        >
          Final
        </button>
      </div>

      <div className={`clip-studio-preview-wrapper aspect-${aspectClass} video-stage-hero`}>
        {videoSrc ? (
          <video
            ref={videoRef as React.Ref<HTMLVideoElement>}
            key={videoSrc}
            src={videoSrc}
            className="clip-studio-video"
            onTimeUpdate={onTimeUpdate}
            onLoadedMetadata={onLoadedMetadata}
            onClick={handleVideoClick}
            onPlay={onPlay}
            onPause={onPause}
          />
        ) : (
          <div className="clip-studio-video clip-studio-video-empty" />
        )}
        {renderGlobalCaptionOverlay()}
        {showCanvasEditor && (
          <CaptionOverlayEditor
            captions={captions}
            currentTime={currentTime}
            selectedCaptionIndex={selectedCaptionIndex}
            addTextMode={addTextMode}
            captionPreset={captionPreset}
            captionSize={captionSize}
            channelEmotes={channelEmotes}
            onSelectCaption={onSelectCaption}
            onUpdateCaption={onUpdateCaption}
            onAddCaptionAt={onAddCaptionAt}
          />
        )}
        {sourceUnavailable && previewMode === 'source' && (
          <div className="clip-studio-unavailable-overlay">
            <h3>{jobState === 'failed' ? 'Clip Creation Failed' : 'Source Video Unavailable'}</h3>
            <p>{failureMessage || 'The raw source file has expired. Re-rendering is no longer available.'}</p>
          </div>
        )}
      </div>
    </section>
  )
}
