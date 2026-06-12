import { useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  getMetadataDiagnostics,
  getSetupControlHealth,
  getSetupStartStatus,
  getSetupWelcome,
  startSetupService,
  type MetadataDiagnostics,
  type SetupWelcome,
} from '../api'

export type ServiceStartProgress = {
  service: 'scraper' | 'clipper'
  percent: number
  phase: string
  detail: string
}

const START_LABELS: Record<'scraper' | 'clipper', string> = {
  scraper: 'Analytics',
  clipper: 'Clip Studio',
}

function serviceReadyFromDiagnostics(
  diagnostics: MetadataDiagnostics | undefined,
  service: 'scraper' | 'clipper',
): boolean {
  if (!diagnostics) return false
  return diagnostics.services[service] === 'ready'
}

function serviceReadyFromWelcome(
  welcome: SetupWelcome | undefined,
  service: 'scraper' | 'clipper',
): boolean {
  if (!welcome) return false
  return welcome.services[service] === 'ready'
}

function isServiceReady(
  welcome: SetupWelcome | undefined,
  diagnostics: MetadataDiagnostics | undefined,
  service: 'scraper' | 'clipper',
): boolean {
  return serviceReadyFromDiagnostics(diagnostics, service) || serviceReadyFromWelcome(welcome, service)
}

export function useOptionalServices() {
  const queryClient = useQueryClient()
  const [starting, setStarting] = useState<'scraper' | 'clipper' | null>(null)
  const [startProgress, setStartProgress] = useState<ServiceStartProgress | null>(null)
  const [actionError, setActionError] = useState<string | null>(null)

  const setup = useQuery({
    queryKey: ['setup-welcome'],
    queryFn: getSetupWelcome,
    staleTime: 10_000,
    refetchInterval: starting ? 2_000 : 15_000,
  })
  const diagnostics = useQuery({
    queryKey: ['setup-diagnostics'],
    queryFn: getMetadataDiagnostics,
    staleTime: 5_000,
    refetchInterval: starting ? 2_000 : 15_000,
  })
  const control = useQuery({
    queryKey: ['setup-control-health'],
    queryFn: getSetupControlHealth,
    staleTime: 10_000,
    retry: false,
  })

  const profile = setup.data?.profile ?? diagnostics.data?.profile ?? 'core'
  const services = useMemo(() => {
    const welcome = setup.data?.services
    const diag = diagnostics.data?.services
    return {
      scraper: diag?.scraper === 'ready' || welcome?.scraper === 'ready'
        ? 'ready'
        : welcome?.scraper ?? diag?.scraper ?? 'offline',
      clipper: diag?.clipper === 'ready' || welcome?.clipper === 'ready'
        ? 'ready'
        : welcome?.clipper ?? diag?.clipper ?? 'offline',
    } as const
  }, [setup.data?.services, diagnostics.data?.services])

  const controlReady = Boolean(control.data?.ok)
  const scraperOffline = services.scraper === 'offline'
  const clipperOffline = services.clipper === 'offline'

  const refreshStatus = async () => {
    setActionError(null)
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['setup-welcome'] }),
      queryClient.invalidateQueries({ queryKey: ['setup-diagnostics'] }),
      queryClient.invalidateQueries({ queryKey: ['setup-control-health'] }),
    ])
  }

  const startService = async (service: 'scraper' | 'clipper') => {
    setActionError(null)
    if (!controlReady) {
      setActionError(
        'The install helper is not running (needed to start Analytics). Close the app tab, run Start Streamclone from Desktop, then try again.',
      )
      return false
    }
    setStarting(service)
    setStartProgress({
      service,
      percent: 8,
      phase: 'Sending start request',
      detail: `Starting ${START_LABELS[service]}...`,
    })
    try {
      await startSetupService(service)
      setStartProgress({
        service,
        percent: 18,
        phase: 'Docker compose running',
        detail: service === 'scraper'
          ? 'First start may build a large browser image (5–15 min).'
          : 'Bringing optional containers online...',
      })
      const maxAttempts = service === 'scraper' ? 450 : 45
      for (let attempt = 0; attempt < maxAttempts; attempt++) {
        await new Promise(resolve => window.setTimeout(resolve, 2000))
        let hostPercent = 18
        let hostPhase = 'Docker compose running'
        let hostDetail = ''
        try {
          const status = await getSetupStartStatus(service)
          hostPercent = Math.min(95, Math.max(18, status.percent))
          hostPhase = status.phase
          hostDetail = status.detail
          if (status.warmup && service === 'scraper') {
            hostDetail = status.warmup
            if (status.phase === 'Warming Camoufox profile') {
              hostPhase = status.phase
            }
          }
          setStartProgress({
            service,
            percent: hostPercent,
            phase: hostPhase,
            detail: hostDetail,
          })
        } catch {
          setStartProgress(prev => prev && prev.service === service
            ? {
                ...prev,
                percent: Math.min(90, prev.percent + 1),
                phase: 'Still starting',
              }
            : prev)
        }
        await Promise.all([
          queryClient.invalidateQueries({ queryKey: ['setup-welcome'] }),
          queryClient.invalidateQueries({ queryKey: ['setup-diagnostics'] }),
        ])
        const welcome = queryClient.getQueryData<SetupWelcome>(['setup-welcome'])
        const diag = queryClient.getQueryData<MetadataDiagnostics>(['setup-diagnostics'])
        const ready = isServiceReady(welcome, diag, service)
        if (ready) {
          setStartProgress({
            service,
            percent: 100,
            phase: 'Ready',
            detail: `${START_LABELS[service]} is online`,
          })
          return true
        }
        if (hostPercent >= 82 && hostPhase === 'Container is up' && service === 'clipper') {
          setStartProgress({
            service,
            percent: 98,
            phase: 'Waiting for health check',
            detail: hostDetail || 'Container is up — confirming API...',
          })
        }
      }
      setActionError(
        service === 'scraper'
          ? 'Analytics is still building or starting. First install can take 5–15 minutes — leave Docker Desktop open and refresh status.'
          : 'Clip Studio is still starting. Check Docker Desktop and retry in a minute.',
      )
      return false
    } catch (err) {
      setActionError(err instanceof Error ? err.message : `Unable to start ${service}.`)
      return false
    } finally {
      setStarting(null)
      window.setTimeout(() => setStartProgress(null), 2500)
    }
  }

  return {
    setup,
    diagnostics,
    control,
    profile,
    services,
    controlReady,
    scraperOffline,
    clipperOffline,
    starting,
    startProgress,
    actionError,
    setActionError,
    startService,
    refreshStatus,
  }
}
