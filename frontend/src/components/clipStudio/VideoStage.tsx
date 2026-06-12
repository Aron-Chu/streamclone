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
  onTimeUpdate: () => void
  onLoadedMetadata: () => void
  onTogglePlay: () => void
  onPlay: () => void
  onPause: () => void
  onSelectCaption: (index: number | null) => void
  onUpdateCaption: (index: number, patch: Partial<CaptionWord>) => void
  onAddCaptionAt: (x: number, y: number) => void
}

function aspectClass(preset: FormatPreset): string {
  if (preset === 'youtube') return 'aspect-video max-h-[min(62vh,720px)]'
  if (preset === 'twitter') return 'aspect-square max-h-[min(62vh,720px)]'
  return 'aspect-[9/16] max-h-[min(72vh,820px)]'
}

function SafeZoneGuides({ preset }: { preset: FormatPreset }) {
  if (preset === 'youtube' || preset === 'twitter') return null
  return (
    <>
      <div className="pointer-events-none absolute inset-x-0 top-0 h-[12%] border-b border-dashed border-white/15" aria-hidden="true" />
      <div className="pointer-events-none absolute inset-x-0 bottom-0 h-[18%] border-t border-dashed border-white/15" aria-hidden="true" />
      <div className="pointer-events-none absolute inset-y-0 left-0 w-[6%] border-r border-dashed border-white/10" aria-hidden="true" />
      <div className="pointer-events-none absolute inset-y-0 right-0 w-[6%] border-l border-dashed border-white/10" aria-hidden="true" />
    </>
  )
}

export function VideoStage({
  videoRef,
  videoSrc,
  previewMode,
  formatPreset,
  canPreviewSource,
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
  onTimeUpdate,
  onLoadedMetadata,
  onTogglePlay,
  onPlay,
  onPause,
  onSelectCaption,
  onUpdateCaption,
  onAddCaptionAt,
}: VideoStageProps) {
  const sourceUnavailable = !canPreviewSource
  const showCanvasEditor = previewMode === 'source' && captionPreset !== 'none'

  const renderGlobalCaptionOverlay = () => {
    if (!showCanvasEditor || !activeCaption || activeCaption.transform) return null
    const posClass = captionPosition === 'top' ? 'top-[14%]' : captionPosition === 'center' ? 'top-1/2 -translate-y-1/2' : 'bottom-[20%]'
    const sizeClass = captionSize === 'sm' ? 'text-sm' : captionSize === 'lg' ? 'text-xl' : 'text-base'
    const isKaraoke = captionPreset === 'karaoke_pop' || captionPreset === 'tiktok_pop'
    return (
      <div className={`pointer-events-none absolute inset-x-4 ${posClass} text-center font-bold drop-shadow-lg ${sizeClass}`}>
        {isKaraoke && activeCaption.words?.length ? (
          activeCaption.words.map((w, i) => (
            <span key={i} className={i === activeWordIndex ? 'text-yellow-300' : 'text-white/90'}>
              <CaptionRichText text={`${w.text} `} emotes={channelEmotes} />
            </span>
          ))
        ) : (
          <CaptionRichText text={activeCaption.text} emotes={channelEmotes} />
        )}
      </div>
    )
  }

  return (
    <section className="flex flex-1 flex-col items-center justify-center bg-[radial-gradient(ellipse_at_center,rgba(34,211,238,0.06),transparent_55%)] p-4">
      <div className={`relative overflow-hidden rounded-xl border border-white/10 bg-black shadow-2xl shadow-black/50 ${aspectClass(formatPreset)} w-auto`}>
        {videoSrc ? (
          <video
            ref={videoRef as React.Ref<HTMLVideoElement>}
            key={videoSrc}
            src={videoSrc}
            className="h-full w-full object-contain"
            onTimeUpdate={onTimeUpdate}
            onLoadedMetadata={onLoadedMetadata}
            onClick={() => { if (!addTextMode) onTogglePlay() }}
            onPlay={onPlay}
            onPause={onPause}
          />
        ) : (
          <div className="flex h-full min-h-[320px] w-full min-w-[180px] items-center justify-center bg-zinc-950 text-sm text-zinc-500">
            No preview
          </div>
        )}
        <SafeZoneGuides preset={formatPreset} />
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
          <div className="absolute inset-0 flex flex-col items-center justify-center bg-black/80 p-6 text-center">
            <h3 className="text-sm font-bold text-rose-300">
              {jobState === 'failed' ? 'Clip creation failed' : 'Source unavailable'}
            </h3>
            <p className="mt-2 max-w-xs text-xs text-zinc-400">{failureMessage || 'Raw source expired.'}</p>
          </div>
        )}
        <div className="pointer-events-none absolute inset-0 rounded-xl ring-1 ring-inset ring-white/5" aria-hidden="true" />
      </div>
    </section>
  )
}
