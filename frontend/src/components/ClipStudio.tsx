import React, { useState, useEffect, useRef, useMemo, useCallback } from 'react'
import { useParams, Link } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import {
  getClipperJob,
  getClipperJobCaptions,
  getClipperTemplates,
  updateClipperJobCaptions,
  renderClipperJob,
  transcribeClipperJob,
  getClipperSourceVideoUrl,
  getClipperFinalVideoUrl,
  describeClipperFailure,
  getChannelEmotes,
  type ClipperJob,
  type CaptionWord,
  type ClipperTemplate,
  type CaptionPreset,
} from '../api'
import { CaptionRichText, CAPTION_EMOJIS } from '../captionTokens'
import './ClipStudio.css'

type CaptionSize = 'sm' | 'md' | 'lg'
type CaptionPosition = 'top' | 'center' | 'bottom'

function formatTime(seconds: number): string {
  const m = Math.floor(seconds / 60)
  const s = seconds % 60
  return `${m}:${s.toFixed(1).padStart(s < 10 ? 4 : 3, '0')}`
}

function parseTimeInput(value: string): number | null {
  if (value.includes(':')) {
    const [min, sec] = value.split(':')
    const parsed = parseFloat(min) * 60 + parseFloat(sec)
    return isNaN(parsed) ? null : parsed
  }
  const parsed = parseFloat(value)
  return isNaN(parsed) ? null : parsed
}

export default function ClipStudio() {
  const { jobId } = useParams<{ jobId: string }>()
  const [job, setJob] = useState<ClipperJob | null>(null)
  const [captions, setCaptions] = useState<CaptionWord[]>([])
  const [templates, setTemplates] = useState<ClipperTemplate[]>([])
  const [selectedTemplateId, setSelectedTemplateId] = useState<string | null>(null)
  const [activeCaption, setActiveCaption] = useState<CaptionWord | null>(null)
  const [activeWordIndex, setActiveWordIndex] = useState(-1)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [toast, setToast] = useState<{ type: 'success' | 'error' | 'info'; message: string } | null>(null)

  const [formatPreset, setFormatPreset] = useState<'tiktok' | 'youtube' | 'youtube_short' | 'instagram_reel' | 'twitter'>('tiktok')
  const [captionPreset, setCaptionPreset] = useState<CaptionPreset>('default')
  const [captionSize, setCaptionSize] = useState<CaptionSize>('md')
  const [captionPosition, setCaptionPosition] = useState<CaptionPosition>('bottom')
  const [showEmojiPicker, setShowEmojiPicker] = useState(false)
  const [emojiTargetIndex, setEmojiTargetIndex] = useState<number | null>(null)
  const [previewMode, setPreviewMode] = useState<'source' | 'final'>('source')

  const [isPlaying, setIsPlaying] = useState(false)
  const [currentTime, setCurrentTime] = useState(0)
  const [duration, setDuration] = useState(0)
  const [trimStart, setTrimStart] = useState(0)
  const [trimEnd, setTrimEnd] = useState(30)

  const [renderStatus, setRenderStatus] = useState<'idle' | 'rendering' | 'success' | 'failed'>('idle')
  const [renderErrorMsg, setRenderErrorMsg] = useState('')
  const [isTranscribing, setIsTranscribing] = useState(false)

  const videoRef = useRef<HTMLVideoElement | null>(null)
  const progressRef = useRef<HTMLDivElement | null>(null)
  const pollingRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const toastTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const showToast = useCallback((type: 'success' | 'error' | 'info', message: string) => {
    setToast({ type, message })
    if (toastTimerRef.current) clearTimeout(toastTimerRef.current)
    toastTimerRef.current = setTimeout(() => setToast(null), 4000)
  }, [])

  const startPolling = useCallback(() => {
    if (pollingRef.current) clearInterval(pollingRef.current)
    pollingRef.current = setInterval(async () => {
      if (!jobId) return
      try {
        const details = await getClipperJob(jobId)
        setJob(details.job)
        if (details.job.state === 'ready') {
          setRenderStatus('success')
          setIsTranscribing(false)
          if (pollingRef.current) clearInterval(pollingRef.current)
          const caps = await getClipperJobCaptions(jobId)
          setCaptions(caps.captions || [])
        } else if (details.job.state === 'failed') {
          setRenderStatus('failed')
          setIsTranscribing(false)
          setRenderErrorMsg(details.job.error_message || 'Processing failed')
          if (pollingRef.current) clearInterval(pollingRef.current)
        } else if (details.job.state === 'transcribing') {
          setIsTranscribing(true)
        }
      } catch (err) {
        console.error('Polling error', err)
      }
    }, 2500)
  }, [jobId])

  const loadJobData = useCallback(async () => {
    if (!jobId) return
    try {
      setLoading(true)
      const [details, caps, tmpl] = await Promise.all([
        getClipperJob(jobId),
        getClipperJobCaptions(jobId),
        getClipperTemplates().catch(() => ({ items: [] as ClipperTemplate[] })),
      ])
      setJob(details.job)
      setTemplates(tmpl.items || [])
      setTrimEnd(details.job.twitch_clip_duration || details.job.source_duration || 30)
      setCaptions(caps.captions || [])

      if (details.job.state === 'rendering' || details.job.state === 'transcribing') {
        if (details.job.state === 'rendering') setRenderStatus('rendering')
        if (details.job.state === 'transcribing') setIsTranscribing(true)
        startPolling()
      }
      setLoading(false)
    } catch (err) {
      console.error(err)
      setError('Failed to load clip details from server')
      setLoading(false)
    }
  }, [jobId, startPolling])

  useEffect(() => {
    loadJobData()
    return () => {
      if (pollingRef.current) clearInterval(pollingRef.current)
      if (toastTimerRef.current) clearTimeout(toastTimerRef.current)
    }
  }, [loadJobData])

  const trimDuration = trimEnd - trimStart

  const channelEmotesQuery = useQuery({
    queryKey: ['clip-studio-emotes', job?.channel],
    queryFn: () => getChannelEmotes(job!.channel),
    enabled: Boolean(job?.channel),
    staleTime: 60_000,
  })
  const channelEmotes = channelEmotesQuery.data ?? []

  const previewRelativeTime = useMemo(() => currentTime - trimStart, [currentTime, trimStart])

  const handleTimeUpdate = () => {
    if (!videoRef.current) return
    const time = videoRef.current.currentTime
    setCurrentTime(time)

    const active = captions.find(c => time >= c.start && time <= c.end)
    setActiveCaption(active || null)

    if (active?.words?.length) {
      const wordIdx = active.words.findIndex(w => time >= w.start && time <= w.end)
      setActiveWordIndex(wordIdx)
    } else {
      setActiveWordIndex(-1)
    }

    if (previewMode === 'source') {
      if (time > trimEnd) videoRef.current.currentTime = trimStart
      if (time < trimStart) videoRef.current.currentTime = trimStart
    }
  }

  const handleLoadedMetadata = () => {
    if (!videoRef.current) return
    const dur = videoRef.current.duration
    setDuration(dur)
    if (trimEnd <= 0 || trimEnd > dur) setTrimEnd(dur)
  }

  const togglePlay = () => {
    if (!videoRef.current) return
    if (isPlaying) {
      videoRef.current.pause()
      setIsPlaying(false)
    } else {
      if (previewMode === 'source' && videoRef.current.currentTime < trimStart) {
        videoRef.current.currentTime = trimStart
      }
      videoRef.current.play()
      setIsPlaying(true)
    }
  }

  const seekTo = (time: number) => {
    if (!videoRef.current) return
    const clamped = Math.max(0, Math.min(time, duration))
    videoRef.current.currentTime = clamped
    setCurrentTime(clamped)
  }

  const handleScrub = (e: React.MouseEvent<HTMLDivElement>) => {
    if (!progressRef.current || duration === 0) return
    const rect = progressRef.current.getBoundingClientRect()
    const percent = (e.clientX - rect.left) / rect.width
    seekTo(percent * duration)
  }

  const applyTemplate = (template: ClipperTemplate) => {
    setSelectedTemplateId(template.id)
    setFormatPreset(template.format_preset as typeof formatPreset)
    setCaptionPreset(template.caption_preset as CaptionPreset)
    showToast('info', `Applied template: ${template.name}`)
  }

  const handleCaptionTextChange = (index: number, newText: string) => {
    const updated = [...captions]
    updated[index].text = newText
    setCaptions(updated)
  }

  const handleCaptionTimeChange = (index: number, field: 'start' | 'end', val: string) => {
    const numeric = parseFloat(val)
    if (isNaN(numeric)) return
    const updated = [...captions]
    updated[index][field] = numeric
    setCaptions(updated)
  }

  const handleSaveCaptions = async () => {
    if (!jobId) return
    try {
      await updateClipperJobCaptions(jobId, captions)
      showToast('success', 'Captions saved')
    } catch (err) {
      console.error(err)
      showToast('error', 'Failed to save captions')
    }
  }

  const handleRetranscribe = async () => {
    if (!jobId) return
    try {
      setIsTranscribing(true)
      const template = templates.find(t => t.id === selectedTemplateId)
      await transcribeClipperJob(jobId, {
        trim_start: trimStart,
        trim_duration: trimDuration,
        max_words_per_line: template?.max_words_per_line ?? 3,
      })
      startPolling()
      showToast('info', 'Re-transcribing trimmed region...')
    } catch (err) {
      console.error(err)
      setIsTranscribing(false)
      showToast('error', 'Failed to start re-transcription')
    }
  }

  const handleAddCaptionRow = () => {
    const newRow: CaptionWord = {
      start: Math.max(trimStart, currentTime),
      end: Math.min(currentTime + 2.5, trimEnd),
      text: 'New subtitle text',
    }
    setCaptions([...captions, newRow].sort((a, b) => a.start - b.start))
  }

  const handleRemoveCaptionRow = (index: number) => {
    setCaptions(captions.filter((_, i) => i !== index))
  }

  const insertEmojiIntoCaption = (index: number, emoji: string) => {
    const updated = [...captions]
    const row = updated[index]
    if (!row) return
    const spacer = row.text && !row.text.endsWith(' ') ? ' ' : ''
    row.text = `${row.text}${spacer}${emoji}`
    setCaptions(updated)
    setShowEmojiPicker(false)
    setEmojiTargetIndex(null)
  }

  const handleExport = async () => {
    if (!jobId) return
    try {
      setRenderStatus('rendering')
      setRenderErrorMsg('')
      await renderClipperJob(jobId, {
        trim_start: trimStart,
        trim_duration: trimDuration,
        format_preset: formatPreset,
        caption_preset: captionPreset,
        template_id: selectedTemplateId || undefined,
      })
      startPolling()
      showToast('info', 'Export started on server')
    } catch (err) {
      console.error(err)
      setRenderStatus('failed')
      setRenderErrorMsg('Failed to trigger export on the server')
      showToast('error', 'Export failed to start')
    }
  }

  const handleTrimInput = (field: 'start' | 'end', value: string) => {
    const parsed = parseTimeInput(value)
    if (parsed === null) return
    if (field === 'start') {
      setTrimStart(Math.max(0, Math.min(parsed, trimEnd - 0.5)))
    } else {
      setTrimEnd(Math.max(trimStart + 0.5, Math.min(parsed, duration || parsed)))
    }
  }

  if (loading) {
    return (
      <div className="clip-studio-container clip-studio-loading">
        <p>Loading Clip Studio Editor...</p>
      </div>
    )
  }

  if (error || !job) {
    return (
      <div className="clip-studio-container clip-studio-loading">
        <h2 className="clip-studio-error-title">Error</h2>
        <p>{error || 'Job not found'}</p>
        <Link to="/analytics" className="clip-studio-back-link">&larr; Back to Analytics</Link>
      </div>
    )
  }

  const canPreviewSource = Boolean(job.raw_path) && job.state !== 'failed' && job.state !== 'purged'
  const canPreviewFinal = job.artifact_available === 1 && job.state === 'ready'
  const sourceUnavailable = !canPreviewSource
  const failureMessage = job.state === 'failed' ? describeClipperFailure(job) : ''
  const aspectClass = formatPreset === 'youtube' ? 'youtube' : formatPreset === 'twitter' ? 'twitter' : 'tiktok'

  const videoSrc = previewMode === 'final' && canPreviewFinal
    ? getClipperFinalVideoUrl(job.id)
    : canPreviewSource
      ? getClipperSourceVideoUrl(job.id)
      : ''

  const renderCaptionOverlay = () => {
    if (captionPreset === 'none' || !activeCaption) return null
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

  return (
    <div className="clip-studio-container">
      {toast && (
        <div className={`clip-studio-toast clip-studio-toast-${toast.type}`}>
          {toast.message}
        </div>
      )}

      <div className="clip-studio-header">
        <div className="clip-studio-header-left">
          <Link to={`/analytics/${job.channel}`} className="clip-studio-back-link">
            <svg width="16" height="16" fill="currentColor" viewBox="0 0 16 16">
              <path fillRule="evenodd" d="M15 8a.5.5 0 0 0-.5-.5H2.707l3.147-3.146a.5.5 0 1 0-.708-.708l-4 4a.5.5 0 0 0 0 .708l4 4a.5.5 0 0 0 .708-.708L2.707 8.5H14.5A.5.5 0 0 0 15 8z"/>
            </svg>
            Back
          </Link>
          <h1>Clip Studio</h1>
          <span className="clip-studio-job-id">#{job.id.substring(0, 8)}</span>
        </div>
        <div className="clip-studio-header-actions">
          {canPreviewSource && (
            <a href={getClipperSourceVideoUrl(job.id)} className="clip-studio-btn-secondary" download>
              Source
            </a>
          )}
          {canPreviewFinal && (
            <a href={getClipperFinalVideoUrl(job.id)} className="clip-studio-btn-secondary" download>
              Final MP4
            </a>
          )}
          <button
            className="btn-export"
            onClick={handleExport}
            disabled={sourceUnavailable || renderStatus === 'rendering'}
          >
            Export
          </button>
        </div>
      </div>

      <div className="clip-studio-workspace">
        <div className="clip-studio-center-panel">
          <div className="clip-studio-preview-toolbar">
            <button
              className={`preview-toggle ${previewMode === 'source' ? 'active' : ''}`}
              onClick={() => setPreviewMode('source')}
              disabled={!canPreviewSource}
            >
              Source
            </button>
            <button
              className={`preview-toggle ${previewMode === 'final' ? 'active' : ''}`}
              onClick={() => setPreviewMode('final')}
              disabled={!canPreviewFinal}
            >
              Final
            </button>
          </div>

          <div className={`clip-studio-preview-wrapper aspect-${aspectClass}`}>
            {videoSrc ? (
              <video
                ref={videoRef}
                key={videoSrc}
                src={videoSrc}
                className="clip-studio-video"
                onTimeUpdate={handleTimeUpdate}
                onLoadedMetadata={handleLoadedMetadata}
                onClick={togglePlay}
                onPlay={() => setIsPlaying(true)}
                onPause={() => setIsPlaying(false)}
              />
            ) : (
              <div className="clip-studio-video clip-studio-video-empty" />
            )}
            {previewMode === 'source' && renderCaptionOverlay()}
          </div>

          {sourceUnavailable && previewMode === 'source' && (
            <div className="clip-studio-unavailable-overlay">
              <h3>{job.state === 'failed' ? 'Clip Creation Failed' : 'Source Video Unavailable'}</h3>
              <p>{failureMessage || 'The raw source file has expired. Re-rendering is no longer available.'}</p>
            </div>
          )}
        </div>

        <div className="clip-studio-right-panel">
          <div className="clip-studio-section">
            <div className="clip-studio-section-title">Templates</div>
            <div className="clip-studio-template-carousel">
              {templates.map(t => (
                <button
                  key={t.id}
                  className={`template-card ${selectedTemplateId === t.id ? 'active' : ''}`}
                  onClick={() => applyTemplate(t)}
                  title={t.description}
                >
                  <span className="template-card-name">{t.name}</span>
                  <span className="template-card-meta">{t.caption_preset}</span>
                </button>
              ))}
            </div>
          </div>

          <div className="clip-studio-section">
            <div className="clip-studio-section-title">Format</div>
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
                  className={`preset-card ${formatPreset === id ? 'active' : ''}`}
                  onClick={() => { setFormatPreset(id); setSelectedTemplateId(null) }}
                >
                  <span>{label}</span>
                </div>
              ))}
            </div>
          </div>

          <div className="clip-studio-section">
            <div className="clip-studio-section-title">Caption Style</div>
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
                  className={`preset-card ${captionPreset === id ? 'active' : ''}`}
                  onClick={() => { setCaptionPreset(id); setSelectedTemplateId(null) }}
                >
                  <span>{label}</span>
                </div>
              ))}
            </div>
            <div className="clip-studio-caption-customize">
              <div className="clip-studio-caption-customize-row">
                <span className="clip-studio-caption-customize-label">Size</span>
                <div className="clip-studio-presets-grid clip-studio-presets-grid-3">
                  {([
                    ['sm', 'Small'],
                    ['md', 'Medium'],
                    ['lg', 'Large'],
                  ] as const).map(([id, label]) => (
                    <div
                      key={id}
                      className={`preset-card ${captionSize === id ? 'active' : ''}`}
                      onClick={() => setCaptionSize(id)}
                    >
                      <span>{label}</span>
                    </div>
                  ))}
                </div>
              </div>
              <div className="clip-studio-caption-customize-row">
                <span className="clip-studio-caption-customize-label">Position</span>
                <div className="clip-studio-presets-grid clip-studio-presets-grid-3">
                  {([
                    ['top', 'Top'],
                    ['center', 'Center'],
                    ['bottom', 'Bottom'],
                  ] as const).map(([id, label]) => (
                    <div
                      key={id}
                      className={`preset-card ${captionPosition === id ? 'active' : ''}`}
                      onClick={() => setCaptionPosition(id)}
                    >
                      <span>{label}</span>
                    </div>
                  ))}
                </div>
              </div>
              {channelEmotes.length > 0 ? (
                <p className="clip-studio-caption-hint">
                  Type channel emote names in captions (e.g. KEKW) to preview 7TV/FFZ images. Burned exports still use text until image captions ship.
                </p>
              ) : null}
            </div>
          </div>

          <div className="clip-studio-section clip-studio-caption-section">
            <div className="clip-studio-section-header">
              <div className="clip-studio-section-title">Captions</div>
              <div className="clip-studio-section-actions">
                <button className="clip-studio-btn-ghost" onClick={handleRetranscribe} disabled={sourceUnavailable || isTranscribing}>
                  {isTranscribing ? 'Transcribing...' : 'Re-transcribe'}
                </button>
                <button className="clip-studio-btn-ghost" onClick={handleSaveCaptions}>Save</button>
              </div>
            </div>

            {showEmojiPicker && emojiTargetIndex !== null ? (
              <div className="clip-studio-emoji-picker">
                {CAPTION_EMOJIS.map(emoji => (
                  <button
                    key={emoji}
                    type="button"
                    className="clip-studio-emoji-btn"
                    onClick={() => insertEmojiIntoCaption(emojiTargetIndex, emoji)}
                  >
                    {emoji}
                  </button>
                ))}
              </div>
            ) : null}

            <div className="clip-studio-caption-list">
              {captions.map((cap, idx) => (
                <div key={idx} className={`caption-row ${activeCaption === cap ? 'active' : ''}`}>
                  <div className="caption-row-times">
                    <input
                      type="number"
                      step="0.1"
                      className="caption-time-input"
                      value={cap.start}
                      onChange={e => handleCaptionTimeChange(idx, 'start', e.target.value)}
                    />
                    <input
                      type="number"
                      step="0.1"
                      className="caption-time-input"
                      value={cap.end}
                      onChange={e => handleCaptionTimeChange(idx, 'end', e.target.value)}
                    />
                  </div>
                  <input
                    type="text"
                    className="caption-text-input"
                    value={cap.text}
                    onChange={e => handleCaptionTextChange(idx, e.target.value)}
                    onClick={() => seekTo(cap.start)}
                  />
                  <button
                    type="button"
                    className="btn-caption-emoji"
                    title="Insert emoji"
                    onClick={() => {
                      setEmojiTargetIndex(idx)
                      setShowEmojiPicker(current => current && emojiTargetIndex === idx ? false : true)
                    }}
                  >
                    😀
                  </button>
                  <button className="btn-remove-caption" onClick={() => handleRemoveCaptionRow(idx)} aria-label="Remove caption">
                    &times;
                  </button>
                </div>
              ))}
            </div>
            <button className="btn-add-caption" onClick={handleAddCaptionRow}>+ Add caption</button>
          </div>
        </div>
      </div>

      <div className="clip-studio-timeline-panel">
        <div className="timeline-trim-inputs">
          <label>
            In
            <input
              type="text"
              value={formatTime(trimStart)}
              onChange={e => handleTrimInput('start', e.target.value)}
              onBlur={e => handleTrimInput('start', e.target.value)}
            />
          </label>
          <label>
            Out
            <input
              type="text"
              value={formatTime(trimEnd)}
              onChange={e => handleTrimInput('end', e.target.value)}
              onBlur={e => handleTrimInput('end', e.target.value)}
            />
          </label>
          <span className="timeline-duration-label">Duration: {trimDuration.toFixed(1)}s</span>
        </div>

        <div className="timeline-scrubber-wrapper" ref={progressRef} onClick={handleScrub}>
          <div className="timeline-scrubber">
            {duration > 0 && captions.map((cap, idx) => (
                <div
                  key={idx}
                  className="timeline-caption-block"
                  style={{
                    left: `${(cap.start / duration) * 100}%`,
                    width: `${Math.max(0.5, ((cap.end - cap.start) / duration) * 100)}%`,
                  }}
                  onClick={e => { e.stopPropagation(); seekTo(cap.start) }}
                  title={cap.text}
                />
            ))}
            <div
              className="timeline-trim-region"
              style={{
                left: `${(trimStart / duration) * 100}%`,
                width: `${((trimEnd - trimStart) / duration) * 100}%`,
              }}
            />
            <div className="timeline-progress" style={{ width: `${duration ? (currentTime / duration) * 100 : 0}%` }} />
            <div className="timeline-playhead" style={{ left: `${duration ? (currentTime / duration) * 100 : 0}%` }} />
            <div
              className="trim-handle trim-handle-left"
              style={{ left: `${duration ? (trimStart / duration) * 100 : 0}%` }}
              onMouseDown={e => {
                e.stopPropagation()
                const onMove = (mv: MouseEvent) => {
                  if (!progressRef.current || !duration) return
                  const rect = progressRef.current.getBoundingClientRect()
                  const pct = Math.max(0, Math.min((mv.clientX - rect.left) / rect.width, trimEnd / duration - 0.01))
                  setTrimStart(pct * duration)
                }
                const onUp = () => {
                  window.removeEventListener('mousemove', onMove)
                  window.removeEventListener('mouseup', onUp)
                }
                window.addEventListener('mousemove', onMove)
                window.addEventListener('mouseup', onUp)
              }}
            />
            <div
              className="trim-handle trim-handle-right"
              style={{ left: `${duration ? (trimEnd / duration) * 100 : 0}%` }}
              onMouseDown={e => {
                e.stopPropagation()
                const onMove = (mv: MouseEvent) => {
                  if (!progressRef.current || !duration) return
                  const rect = progressRef.current.getBoundingClientRect()
                  const pct = Math.max(trimStart / duration + 0.01, Math.min((mv.clientX - rect.left) / rect.width, 1))
                  setTrimEnd(pct * duration)
                }
                const onUp = () => {
                  window.removeEventListener('mousemove', onMove)
                  window.removeEventListener('mouseup', onUp)
                }
                window.addEventListener('mousemove', onMove)
                window.addEventListener('mouseup', onUp)
              }}
            />
          </div>
        </div>

        <div className="timeline-stats">
          <span>Playhead: {formatTime(currentTime)} / {formatTime(duration)}</span>
          <span>Export window: {formatTime(trimStart)} &rarr; {formatTime(trimEnd)}</span>
          {previewMode === 'source' && <span>Caption rel: {previewRelativeTime.toFixed(1)}s</span>}
        </div>

        <div className="timeline-controls">
          <button className="btn-circle" onClick={() => seekTo(trimStart)} title="Go to trim start">
            <svg width="16" height="16" fill="currentColor" viewBox="0 0 16 16"><path d="M4 4a.5.5 0 0 1 1 0v3.248l6.267-3.636c.54-.313 1.232.066 1.232.696v7.384c0 .63-.692 1.01-1.232.697L5 8.753V12a.5.5 0 0 1-1 0V4z"/></svg>
          </button>
          <button className="btn-circle play" onClick={togglePlay}>
            {isPlaying ? (
              <svg width="18" height="18" fill="currentColor" viewBox="0 0 16 16"><path d="M5.5 3.5A1.5 1.5 0 0 1 7 5v6a1.5 1.5 0 0 1-3 0V5a1.5 1.5 0 0 1 1.5-1.5zm5 0A1.5 1.5 0 0 1 12 5v6a1.5 1.5 0 0 1-3 0V5a1.5 1.5 0 0 1 1.5-1.5z"/></svg>
            ) : (
              <svg width="18" height="18" fill="currentColor" viewBox="0 0 16 16" style={{ marginLeft: '2px' }}><path d="M11.596 8.697l-6.363 3.692c-.54.313-1.233-.066-1.233-.697V4.308c0-.63.692-1.01 1.233-.696l6.363 3.692a.802.802 0 0 1 0 1.393z"/></svg>
            )}
          </button>
          <button className="btn-circle" onClick={() => seekTo(trimEnd)} title="Go to trim end">
            <svg width="16" height="16" fill="currentColor" viewBox="0 0 16 16"><path d="M12.5 4a.5.5 0 0 0-1 0v3.248L5.233 3.612C4.693 3.3 4 3.678 4 4.308v7.384c0 .63.692 1.01 1.233.697L11.5 8.753V12a.5.5 0 0 0 1 0V4z"/></svg>
          </button>
        </div>

        {renderStatus !== 'idle' && (
          <div className="clip-studio-progress-card">
            <span className={`progress-status progress-status-${renderStatus}`}>
              {renderStatus === 'rendering' && 'Rendering on server...'}
              {renderStatus === 'success' && 'Render complete'}
              {renderStatus === 'failed' && 'Render failed'}
            </span>
            {renderStatus === 'failed' && <p className="progress-error">{renderErrorMsg}</p>}
            {renderStatus === 'success' && canPreviewFinal && (
              <a href={getClipperFinalVideoUrl(job.id)} className="btn-download" download>
                Download rendered MP4
              </a>
            )}
          </div>
        )}
      </div>
    </div>
  )
}
