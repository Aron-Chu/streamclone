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
import { SETUP_CONTROL_AVAILABLE } from '../config'

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

export function useOptionalServices(options: { probeControl?: boolean; pollActive?: boolean } = {}) {
  const queryClient = useQueryClient()
  const [starting, setStarting] = useState<Set<'scraper' | 'clipper'>>(() => new Set())
  const [startProgressByService, setStartProgressByService] = useState<
    Partial<Record<'scraper' | 'clipper', ServiceStartProgress>>
  >({})
  const [actionError, setActionError] = useState<string | null>(null)

  const anyStarting = starting.size > 0
  const pollActive = options.pollActive ?? false
  const statusPollMs = anyStarting ? 2_000 : pollActive ? 15_000 : 60_000
  const startProgress = startProgressByService.scraper ?? startProgressByService.clipper ?? null

  const setup = useQuery({
    queryKey: ['setup-welcome'],
    queryFn: getSetupWelcome,
    staleTime: 10_000,
    refetchInterval: statusPollMs,
    retry: false,
  })
  const diagnostics = useQuery({
    queryKey: ['setup-diagnostics'],
    queryFn: getMetadataDiagnostics,
    staleTime: 5_000,
    refetchInterval: statusPollMs,
    retry: false,
  })
  const control = useQuery({
    queryKey: ['setup-control-health'],
    queryFn: getSetupControlHealth,
    enabled: SETUP_CONTROL_AVAILABLE && (Boolean(options.probeControl) || anyStarting),
    staleTime: 10_000,
    retry: false,
  })

  const hasServiceSnapshot = Boolean(setup.data || diagnostics.data)
  const statusLoading = !hasServiceSnapshot && (setup.isLoading || diagnostics.isLoading)
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
      SETUP_CONTROL_AVAILABLE && (Boolean(options.probeControl) || anyStarting)
        ? queryClient.invalidateQueries({ queryKey: ['setup-control-health'] })
        : Promise.resolve(),
    ])
  }

  const startService = async (service: 'scraper' | 'clipper') => {
    setActionError(null)
    if (!controlReady) {
      setActionError(
        `The install helper is not running (needed to start ${START_LABELS[service]}). Close the app tab, run Start Streamclone from Desktop, then try again.`,
      )
      return false
    }
    setStarting(prev => new Set(prev).add(service))
    setStartProgressByService(prev => ({
      ...prev,
      [service]: {
        service,
        percent: 8,
        phase: 'Sending start request',
        detail: `Starting ${START_LABELS[service]}...`,
      },
    }))
    try {
      const startResp = await startSetupService(service)
      if (startResp.message?.includes('queued')) {
        setStartProgressByService(prev => ({
          ...prev,
          [service]: {
            service,
            percent: 12,
            phase: 'Queued',
            detail: startResp.message,
          },
        }))
      } else {
        setStartProgressByService(prev => ({
          ...prev,
          [service]: {
            service,
            percent: 18,
            phase: 'Docker compose running',
            detail: service === 'scraper'
              ? 'First start may build a large browser image (5–15 min).'
              : 'Bringing optional containers online...',
          },
        }))
      }
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
          setStartProgressByService(prev => ({
            ...prev,
            [service]: {
              service,
              percent: hostPercent,
              phase: hostPhase,
              detail: hostDetail || prev[service]?.detail || '',
            },
          }))
        } catch {
          setStartProgressByService(prev => {
            const cur = prev[service]
            if (!cur) return prev
            return {
              ...prev,
              [service]: {
                ...cur,
                percent: Math.min(90, cur.percent + 1),
                phase: 'Still starting',
              },
            }
          })
        }
        await Promise.all([
          queryClient.invalidateQueries({ queryKey: ['setup-welcome'] }),
          queryClient.invalidateQueries({ queryKey: ['setup-diagnostics'] }),
        ])
        const welcome = queryClient.getQueryData<SetupWelcome>(['setup-welcome'])
        const diag = queryClient.getQueryData<MetadataDiagnostics>(['setup-diagnostics'])
        const ready = isServiceReady(welcome, diag, service)
        if (ready) {
          setStartProgressByService(prev => ({
            ...prev,
            [service]: {
              service,
              percent: 100,
              phase: 'Ready',
              detail: `${START_LABELS[service]} is online`,
            },
          }))
          return true
        }
        if (hostPercent >= 82 && hostPhase === 'Container is up' && service === 'clipper') {
          setStartProgressByService(prev => ({
            ...prev,
            [service]: {
              service,
              percent: 98,
              phase: 'Waiting for health check',
              detail: hostDetail || 'Container is up — confirming API...',
            },
          }))
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
      setStarting(prev => {
        const next = new Set(prev)
        next.delete(service)
        return next
      })
      window.setTimeout(() => {
        setStartProgressByService(prev => {
          if (!prev[service]) return prev
          const next = { ...prev }
          delete next[service]
          return next
        })
      }, 2500)
    }
  }

  const isStarting = (service: 'scraper' | 'clipper') => starting.has(service)

  return {
    setup,
    diagnostics,
    control,
    hasServiceSnapshot,
    statusLoading,
    profile,
    services,
    controlReady,
    scraperOffline,
    clipperOffline,
    starting: isStarting('scraper') ? 'scraper' : isStarting('clipper') ? 'clipper' : null,
    startingServices: starting,
    startProgress,
    startProgressByService,
    actionError,
    setActionError,
    startService,
    refreshStatus,
    isStarting,
  }
}
