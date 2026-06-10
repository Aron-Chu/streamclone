import { useCallback, useEffect, useRef, useState } from 'react'
import { CaptionRichText } from '../../captionTokens'
import type {
  CaptionEffect,
  CaptionPreset,
  CaptionSize,
  CaptionTransform,
  CaptionWord,
  ChannelEmote,
} from '../../api'

interface CaptionOverlayEditorProps {
  captions: CaptionWord[]
  currentTime: number
  selectedCaptionIndex: number | null
  addTextMode: boolean
  captionPreset: CaptionPreset
  captionSize: CaptionSize
  channelEmotes: ChannelEmote[]
  onSelectCaption: (index: number | null) => void
  onUpdateCaption: (index: number, patch: Partial<CaptionWord>) => void
  onAddCaptionAt: (x: number, y: number) => void
}

type DragMode = 'move' | 'scale' | 'rotate'

interface DragState {
  mode: DragMode
  index: number
  startX: number
  startY: number
  origin: CaptionTransform
}

const DEFAULT_TRANSFORM: CaptionTransform = { x: 0.5, y: 0.85, rotation: 0, scale: 1 }

function effectClass(effect?: CaptionEffect): string {
  if (!effect || effect === 'none') return ''
  return `caption-effect-${effect}`
}

function pointerToNormalized(layer: HTMLElement, clientX: number, clientY: number) {
  const rect = layer.getBoundingClientRect()
  return {
    x: Math.max(0, Math.min(1, (clientX - rect.left) / rect.width)),
    y: Math.max(0, Math.min(1, (clientY - rect.top) / rect.height)),
  }
}

export function CaptionOverlayEditor({
  captions,
  currentTime,
  selectedCaptionIndex,
  addTextMode,
  captionPreset,
  captionSize,
  channelEmotes,
  onSelectCaption,
  onUpdateCaption,
  onAddCaptionAt,
}: CaptionOverlayEditorProps) {
  const layerRef = useRef<HTMLDivElement>(null)
  const [drag, setDrag] = useState<DragState | null>(null)

  const visible = captions
    .map((cap, index) => ({ cap, index }))
    .filter(({ cap }) => cap.transform && currentTime >= cap.start && currentTime <= cap.end)

  const finishDrag = useCallback(() => setDrag(null), [])

  useEffect(() => {
    if (!drag) return

    const onMove = (e: MouseEvent) => {
      const layer = layerRef.current
      if (!layer) return
      const cap = captions[drag.index]
      const transform = cap.transform ?? DEFAULT_TRANSFORM
      const pos = pointerToNormalized(layer, e.clientX, e.clientY)
      const dx = pos.x - drag.startX
      const dy = pos.y - drag.startY

      if (drag.mode === 'move') {
        onUpdateCaption(drag.index, {
          transform: {
            ...transform,
            x: Math.max(0, Math.min(1, drag.origin.x + dx)),
            y: Math.max(0, Math.min(1, drag.origin.y + dy)),
          },
        })
        return
      }

      if (drag.mode === 'scale') {
        const delta = dy * -0.02
        onUpdateCaption(drag.index, {
          transform: {
            ...transform,
            scale: Math.max(0.4, Math.min(3, drag.origin.scale + delta)),
          },
        })
        return
      }

      const centerX = transform.x * layer.clientWidth
      const centerY = transform.y * layer.clientHeight
      const angle = Math.atan2(e.clientY - layer.getBoundingClientRect().top - centerY, e.clientX - layer.getBoundingClientRect().left - centerX)
      const degrees = (angle * 180) / Math.PI + 90
      onUpdateCaption(drag.index, {
        transform: { ...transform, rotation: degrees },
      })
    }

    const onUp = () => finishDrag()
    window.addEventListener('mousemove', onMove)
    window.addEventListener('mouseup', onUp)
    return () => {
      window.removeEventListener('mousemove', onMove)
      window.removeEventListener('mouseup', onUp)
    }
  }, [captions, drag, finishDrag, onUpdateCaption])

  const handleLayerClick = (e: React.MouseEvent<HTMLDivElement>) => {
    if (drag) return
    const target = e.target as HTMLElement
    if (target.closest('.caption-overlay-item') || target.closest('.caption-overlay-handle')) return

    const layer = layerRef.current
    if (!layer) return
    const pos = pointerToNormalized(layer, e.clientX, e.clientY)

    if (addTextMode) {
      if (selectedCaptionIndex !== null) {
        const existing = captions[selectedCaptionIndex]?.transform ?? DEFAULT_TRANSFORM
        onUpdateCaption(selectedCaptionIndex, { transform: { ...existing, x: pos.x, y: pos.y } })
      } else {
        onAddCaptionAt(pos.x, pos.y)
      }
      return
    }
    onSelectCaption(null)
  }

  const startDrag = (e: React.MouseEvent, mode: DragMode, index: number) => {
    e.stopPropagation()
    const layer = layerRef.current
    const cap = captions[index]
    if (!layer || !cap?.transform) return
    const pos = pointerToNormalized(layer, e.clientX, e.clientY)
    onSelectCaption(index)
    setDrag({ mode, index, startX: pos.x, startY: pos.y, origin: { ...cap.transform } })
  }

  return (
    <div
      ref={layerRef}
      className={`caption-overlay-layer${addTextMode ? ' caption-overlay-layer-place' : ''}`}
      onClick={handleLayerClick}
    >
      {addTextMode && (
        <div className="caption-overlay-place-hint">
          {selectedCaptionIndex !== null ? 'Click to place selected caption' : 'Click to add caption on canvas'}
        </div>
      )}
      {visible.map(({ cap, index }) => {
        const transform = cap.transform!
        const selected = selectedCaptionIndex === index
        const style = {
          left: `${transform.x * 100}%`,
          top: `${transform.y * 100}%`,
          transform: `translate(-50%, -50%) rotate(${transform.rotation}deg) scale(${transform.scale})`,
        }
        return (
          <div
            key={index}
            className={[
              'caption-overlay-item',
              `subtitle-overlay-${captionPreset}`,
              `caption-size-${captionSize}`,
              effectClass(cap.effect),
              selected ? 'caption-overlay-item-selected' : '',
            ].join(' ')}
            style={style}
            onClick={e => {
              e.stopPropagation()
              onSelectCaption(index)
            }}
          >
            <div className={`caption-overlay-content ${effectClass(cap.effect)}`}>
              <CaptionRichText text={cap.text} emotes={channelEmotes} />
            </div>
            {selected && (
              <>
                <button
                  type="button"
                  className="caption-overlay-handle caption-overlay-handle-move"
                  aria-label="Move caption"
                  onMouseDown={e => startDrag(e, 'move', index)}
                />
                <button
                  type="button"
                  className="caption-overlay-handle caption-overlay-handle-scale"
                  aria-label="Scale caption"
                  onMouseDown={e => startDrag(e, 'scale', index)}
                />
                <button
                  type="button"
                  className="caption-overlay-handle caption-overlay-handle-rotate"
                  aria-label="Rotate caption"
                  onMouseDown={e => startDrag(e, 'rotate', index)}
                />
              </>
            )}
          </div>
        )
      })}
    </div>
  )
}
