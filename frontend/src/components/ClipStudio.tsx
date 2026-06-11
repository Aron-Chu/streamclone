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
  retryClipperJob,
  isClipperJobInProgress,
  getChannelEmotes,
  ensureChannelEmotes,
  getChannel,
  type CaptionWord,
  type CaptionEffect,
  type CaptionTransform,
  type ClipperJob,
  type ClipperTemplate,
  type CaptionPreset,
  type CaptionSize,
  type CaptionPosition,
} from '../api'
import { sortChannelEmotesByUsage } from '../emoteUtils'
import { ClipDetailsPanel } from './clipStudio/ClipDetailsPanel'
import { ClipInspector } from './clipStudio/ClipInspector'
import { ClipTimeline } from './clipStudio/ClipTimeline'
import { JobProgressOverlay } from './clipStudio/JobProgressOverlay'
import { StudioTopBar } from './clipStudio/StudioTopBar'
import { VideoStage } from './clipStudio/VideoStage'
import type { FormatPreset, InspectorTab, PreviewMode, RenderStatus } from './clipStudio/types'
import { buildEmoteMap, buildUploadPackage, parseTimeInput, spikePositionInSource } from './clipStudio/utils'
import OptionalServicesPanel from './OptionalServicesPanel'
import StackStatusButton from './StackStatusButton'
import './ClipStudio.css'

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
  const [toast, setToast] = useState<{ type: 'success' | 'error' | 'info'; message: string; phase: 'visible' | 'exiting' } | null>(null)
  const [selectedCaptionIndex, setSelectedCaptionIndex] = useState<number | null>(null)
  const [addTextMode, setAddTextMode] = useState(false)
  const [inspectorTab, setInspectorTab] = useState<InspectorTab>('template')

  const [formatPreset, setFormatPreset] = useState<FormatPreset>('tiktok')
  const [captionPreset, setCaptionPreset] = useState<CaptionPreset>('default')
  const [captionSize, setCaptionSize] = useState<CaptionSize>('md')
  const [captionPosition, setCaptionPosition] = useState<CaptionPosition>('bottom')
  const [layout, setLayout] = useState('blur_bg_center')
  const [layoutSplitRatio, setLayoutSplitRatio] = useState(0.35)
  const [showEmojiPicker, setShowEmojiPicker] = useState(false)
  const [emojiTargetIndex, setEmojiTargetIndex] = useState<number | null>(null)
  const [previewMode, setPreviewMode] = useState<PreviewMode>('source')

  const [isPlaying, setIsPlaying] = useState(false)
  const [currentTime, setCurrentTime] = useState(0)
  const [duration, setDuration] = useState(0)
  const [trimStart, setTrimStart] = useState(0)
  const [trimEnd, setTrimEnd] = useState(30)

  const [renderStatus, setRenderStatus] = useState<RenderStatus>('idle')
  const [renderErrorMsg, setRenderErrorMsg] = useState('')
  const [isTranscribing, setIsTranscribing] = useState(false)

  const videoRef = useRef<HTMLVideoElement | null>(null)
  const progressRef = useRef<HTMLDivElement | null>(null)
  const pollingRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const toastTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const lastCaptionPresetRef = useRef<CaptionPreset>('default')

  const dismissToast = useCallback(() => {
    setToast(prev => (prev ? { ...prev, phase: 'exiting' } : null))
  }, [])

  const showToast = useCallback((type: 'success' | 'error' | 'info', message: string) => {
    if (toastTimerRef.current) clearTimeout(toastTimerRef.current)
    setToast({ type, message, phase: 'visible' })
    toastTimerRef.current = setTimeout(dismissToast, 3500)
  }, [dismissToast])

  const handleToastAnimationEnd = () => {
    setToast(prev => (prev?.phase === 'exiting' ? null : prev))
  }

  const startPolling = useCallback(() => {
    if (pollingRef.current) clearInterval(pollingRef.current)
    pollingRef.current = setInterval(async () => {
      if (!jobId) return
      try {
        const details = await getClipperJob(jobId)
        setJob(details.job)
        if (details.job.state === 'ready') {
          setIsTranscribing(false)
          if (pollingRef.current) clearInterval(pollingRef.current)
          setRenderStatus(prev => (prev === 'rendering' ? 'success' : prev))
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

      if (isClipperJobInProgress(details.job) || details.job.state === 'rendering' || details.job.state === 'transcribing') {
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

  useEffect(() => {
    if (!job?.channel) return
    const ensure = (twitchId: string) => {
      ensureChannelEmotes(job.channel, twitchId, ['seventv', 'twitch', 'ffz']).catch(() => undefined)
    }
    if (job.broadcaster_id) {
      ensure(job.broadcaster_id)
      return
    }
    getChannel(job.channel)
      .then(channel => ensure(channel.id))
      .catch(() => undefined)
  }, [job?.channel, job?.broadcaster_id])

  const trimDuration = trimEnd - trimStart

  const channelEmotesQuery = useQuery({
    queryKey: ['clip-studio-emotes', job?.channel],
    queryFn: () => getChannelEmotes(job!.channel),
    enabled: Boolean(job?.channel),
    staleTime: 60_000,
  })
  const channelEmotes = useMemo(
    () => sortChannelEmotesByUsage(channelEmotesQuery.data ?? []),
    [channelEmotesQuery.data],
  )

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
    setFormatPreset(template.format_preset as FormatPreset)
    setCaptionPreset(template.caption_preset as CaptionPreset)
    if (template.layout) setLayout(template.layout)
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
    const next = [...captions, newRow].sort((a, b) => a.start - b.start)
    setCaptions(next)
    setSelectedCaptionIndex(next.indexOf(newRow))
  }

  const handleAddCaptionAt = (x: number, y: number) => {
    const transform: CaptionTransform = { x, y, rotation: 0, scale: 1 }
    const newRow: CaptionWord = {
      start: Math.max(trimStart, currentTime),
      end: Math.min(currentTime + 2.5, trimEnd),
      text: 'Text',
      transform,
    }
    const next = [...captions, newRow].sort((a, b) => a.start - b.start)
    setCaptions(next)
    setSelectedCaptionIndex(next.indexOf(newRow))
    setAddTextMode(false)
  }

  const handleUpdateCaption = (index: number, patch: Partial<CaptionWord>) => {
    const updated = [...captions]
    updated[index] = { ...updated[index], ...patch }
    setCaptions(updated)
  }

  const handleCaptionEffectChange = (index: number, effect: CaptionEffect) => {
    handleUpdateCaption(index, { effect })
  }

  const handleResetCaptionPosition = (index: number) => {
    const updated = [...captions]
    const row = { ...updated[index] }
    delete row.transform
    updated[index] = row
    setCaptions(updated)
  }

  const handleRemoveCaptionRow = (index: number) => {
    setCaptions(captions.filter((_, i) => i !== index))
    setSelectedCaptionIndex(prev => {
      if (prev === null) return null
      if (prev === index) return null
      if (prev > index) return prev - 1
      return prev
    })
  }

  const insertIntoCaption = (index: number, token: string) => {
    const updated = [...captions]
    const row = updated[index]
    if (!row) return
    const spacer = row.text && !row.text.endsWith(' ') ? ' ' : ''
    row.text = `${row.text}${spacer}${token}`
    setCaptions(updated)
    setShowEmojiPicker(false)
    setEmojiTargetIndex(null)
  }

  const insertEmojiIntoCaption = (index: number, emoji: string) => {
    insertIntoCaption(index, emoji)
  }

  const insertEmoteIntoCaption = (index: number, emoteName: string) => {
    insertIntoCaption(index, emoteName)
  }

  const handleRetryJob = async () => {
    if (!jobId) return
    try {
      setRenderStatus('idle')
      setRenderErrorMsg('')
      const result = await retryClipperJob(jobId)
      const nextJob = result.job
      if (result.job.id !== jobId) {
        window.location.href = `/studio/${result.job.id}`
        return
      }
      setJob(nextJob)
      startPolling()
      showToast('info', 'Clip job re-queued — source download will resume automatically.')
    } catch (err) {
      console.error(err)
      showToast('error', 'Failed to retry clip job')
    }
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
        caption_size: captionSize,
        caption_position: captionPosition,
        layout,
        layout_split_ratio: layoutSplitRatio,
        emote_map: buildEmoteMap(channelEmotes),
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

  const applyDurationPreset = (seconds: number) => {
    const spike = job ? spikePositionInSource(job, duration) : null
    const anchor = spike ?? (trimStart + trimDuration / 2)
    const half = seconds / 2
    const start = Math.max(0, anchor - half)
    const end = Math.min(duration || anchor + half, start + seconds)
    setTrimStart(start)
    setTrimEnd(end)
    showToast('info', `Trim set to ${seconds}s window`)
  }

  const applyTrimWindow = (window: 'setup' | 'spike' | 'payoff' | 'full') => {
    const spike = job ? spikePositionInSource(job, duration) : null
    const anchor = spike ?? (trimStart + trimDuration / 2)
    if (window === 'setup') {
      setTrimStart(Math.max(0, anchor - 18))
      setTrimEnd(Math.min(duration || anchor, anchor))
    } else if (window === 'spike') {
      setTrimStart(Math.max(0, anchor - 5))
      setTrimEnd(Math.min(duration || anchor + 5, anchor + 5))
      seekTo(anchor)
    } else if (window === 'payoff') {
      setTrimStart(anchor)
      setTrimEnd(Math.min(duration || anchor + 18, anchor + 18))
    } else {
      setTrimStart(Math.max(0, anchor - 15))
      setTrimEnd(Math.min(duration || anchor + 15, anchor + 15))
    }
    showToast('info', `Applied ${window} trim window`)
  }

  const handleCaptionsToggle = (enabled: boolean) => {
    if (enabled) {
      setCaptionPreset(lastCaptionPresetRef.current === 'none' ? 'default' : lastCaptionPresetRef.current)
    } else {
      if (captionPreset !== 'none') lastCaptionPresetRef.current = captionPreset
      setCaptionPreset('none')
    }
    setSelectedTemplateId(null)
  }

  const handleFacecamToggle = (enabled: boolean) => {
    setLayout(enabled ? 'stacked_game_face' : 'blur_bg_center')
  }

  const handleCaptionPresetChange = (preset: CaptionPreset) => {
    if (preset !== 'none') lastCaptionPresetRef.current = preset
    setCaptionPreset(preset)
    setSelectedTemplateId(null)
  }

  const handleFormatPresetChange = (preset: FormatPreset) => {
    setFormatPreset(preset)
    setSelectedTemplateId(null)
  }

  const handleCopyUploadPackage = async () => {
    if (!job) return
    const text = buildUploadPackage(job, trimDuration)
    try {
      await navigator.clipboard.writeText(text)
      showToast('success', 'Upload package copied to clipboard')
    } catch {
      showToast('error', 'Could not copy to clipboard')
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
    const analyticsBack = job?.channel
      ? `/analytics/${encodeURIComponent(job.channel)}`
      : '/'
    return (
      <div className="clip-studio-container clip-studio-loading">
        <div className="mb-4 flex flex-wrap items-center gap-2">
          <StackStatusButton />
          <Link to="/" className="clip-studio-back-link">Live directory</Link>
          {job?.channel ? (
            <Link to={analyticsBack} className="clip-studio-back-link">&larr; Back to Analytics</Link>
          ) : null}
        </div>
        <h2 className="clip-studio-error-title">Error</h2>
        <p>{error || 'Job not found'}</p>
        <div className="mt-4 max-w-xl">
          <OptionalServicesPanel variant="banner" focus="clipper" channelLogin={job?.channel} />
        </div>
      </div>
    )
  }

  const canPreviewSource = Boolean(job.raw_path) && job.state !== 'failed' && job.state !== 'purged'
  const canPreviewFinal = job.artifact_available === 1 && job.state === 'ready'
  const sourceUnavailable = !canPreviewSource
  const failureMessage = job.state === 'failed' ? describeClipperFailure(job) : ''

  const videoSrc = previewMode === 'final' && canPreviewFinal
    ? getClipperFinalVideoUrl(job.id)
    : canPreviewSource
      ? getClipperSourceVideoUrl(job.id)
      : ''

  return (
    <div className="clip-studio-container">
      {toast && (
        <div
          className={`clip-studio-toast clip-studio-toast-${toast.type}${toast.phase === 'exiting' ? ' clip-studio-toast-out' : ''}`}
          onClick={dismissToast}
          onAnimationEnd={handleToastAnimationEnd}
          role="status"
        >
          {toast.message}
        </div>
      )}

      <StudioTopBar
        job={job}
        trimStart={trimStart}
        trimEnd={trimEnd}
        duration={duration}
        canPreviewSource={canPreviewSource}
        canPreviewFinal={canPreviewFinal}
        sourceUrl={getClipperSourceVideoUrl(job.id)}
        finalUrl={getClipperFinalVideoUrl(job.id)}
        onExport={handleExport}
        exportDisabled={sourceUnavailable || renderStatus === 'rendering'}
      />

      <div className="clip-studio-workspace">
        <ClipDetailsPanel
          job={job}
          trimStart={trimStart}
          trimEnd={trimEnd}
          trimDuration={trimDuration}
          captionPreset={captionPreset}
          layout={layout}
          captionsCount={captions.length}
          isTranscribing={isTranscribing}
          onToggleCaptions={handleCaptionsToggle}
          onToggleFacecamFocus={handleFacecamToggle}
          onCenterOnSpike={() => applyTrimWindow('payoff')}
          onApplyDurationPreset={applyDurationPreset}
          onOpenCaptionsTab={() => setInspectorTab('captions')}
        />

        <div className="clip-studio-center-panel">
          <VideoStage
            videoRef={videoRef}
            videoSrc={videoSrc}
            previewMode={previewMode}
            formatPreset={formatPreset}
            canPreviewSource={canPreviewSource}
            canPreviewFinal={canPreviewFinal}
            failureMessage={failureMessage}
            jobState={job.state}
            captionPreset={captionPreset}
            captionSize={captionSize}
            captionPosition={captionPosition}
            activeCaption={activeCaption}
            activeWordIndex={activeWordIndex}
            channelEmotes={channelEmotes}
            captions={captions}
            currentTime={currentTime}
            selectedCaptionIndex={selectedCaptionIndex}
            addTextMode={addTextMode}
            onSelectCaption={setSelectedCaptionIndex}
            onUpdateCaption={handleUpdateCaption}
            onAddCaptionAt={handleAddCaptionAt}
            onFormatPresetChange={handleFormatPresetChange}
            onPreviewModeChange={setPreviewMode}
            onTimeUpdate={handleTimeUpdate}
            onLoadedMetadata={handleLoadedMetadata}
            onTogglePlay={togglePlay}
            onPlay={() => setIsPlaying(true)}
            onPause={() => setIsPlaying(false)}
          />
        </div>

        <ClipInspector
          activeTab={inspectorTab}
          onTabChange={setInspectorTab}
          templates={templates}
          selectedTemplateId={selectedTemplateId}
          formatPreset={formatPreset}
          captionPreset={captionPreset}
          captionSize={captionSize}
          captionPosition={captionPosition}
          layout={layout}
          layoutSplitRatio={layoutSplitRatio}
          captions={captions}
          activeCaption={activeCaption}
          selectedCaptionIndex={selectedCaptionIndex}
          addTextMode={addTextMode}
          channelEmotes={channelEmotes}
          job={job}
          trimStart={trimStart}
          trimEnd={trimEnd}
          duration={duration}
          sourceUnavailable={sourceUnavailable}
          isTranscribing={isTranscribing}
          showEmojiPicker={showEmojiPicker}
          emojiTargetIndex={emojiTargetIndex}
          onApplyTemplate={applyTemplate}
          onFormatPresetChange={handleFormatPresetChange}
          onCaptionPresetChange={handleCaptionPresetChange}
          onCaptionSizeChange={setCaptionSize}
          onCaptionPositionChange={setCaptionPosition}
          onLayoutChange={setLayout}
          onLayoutSplitRatioChange={setLayoutSplitRatio}
          onRetranscribe={handleRetranscribe}
          onSaveCaptions={handleSaveCaptions}
          onAddCaptionRow={handleAddCaptionRow}
          onCaptionTextChange={handleCaptionTextChange}
          onCaptionTimeChange={handleCaptionTimeChange}
          onRemoveCaptionRow={handleRemoveCaptionRow}
          onSeekToCaption={seekTo}
          onSelectCaption={setSelectedCaptionIndex}
          onAddTextModeChange={setAddTextMode}
          onCaptionEffectChange={handleCaptionEffectChange}
          onResetCaptionPosition={handleResetCaptionPosition}
          onEmojiPickerToggle={idx => {
            setEmojiTargetIndex(idx)
            setShowEmojiPicker(current => current && emojiTargetIndex === idx ? false : true)
          }}
          onInsertEmoji={insertEmojiIntoCaption}
          onInsertEmote={insertEmoteIntoCaption}
          onCopyUploadPackage={handleCopyUploadPackage}
        />
      </div>

      <ClipTimeline
        progressRef={progressRef}
        captions={captions}
        duration={duration}
        currentTime={currentTime}
        trimStart={trimStart}
        trimEnd={trimEnd}
        isPlaying={isPlaying}
        job={job}
        previewRelativeTime={previewRelativeTime}
        previewMode={previewMode}
        onScrub={handleScrub}
        onTrimInput={handleTrimInput}
        onSeekTo={seekTo}
        onTogglePlay={togglePlay}
        onTrimStartChange={setTrimStart}
        onTrimEndChange={setTrimEnd}
        onApplyTrimWindow={applyTrimWindow}
      />

      <JobProgressOverlay
        job={job}
        renderStatus={renderStatus}
        renderErrorMsg={renderErrorMsg}
        isTranscribing={isTranscribing}
        canPreviewFinal={canPreviewFinal}
        finalUrl={getClipperFinalVideoUrl(job.id)}
        failureMessage={failureMessage}
        onRetry={job.state === 'failed' ? handleRetryJob : undefined}
      />
    </div>
  )
}
