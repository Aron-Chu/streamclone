import { useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  getHostDiagnostics,
  getMetadataDiagnostics,
  getSetupControlHealth,
  getSetupWelcome,
} from '../api'
import { SETUP_CONTROL_AVAILABLE, SETUP_CONTROL_WAKE_ENABLED } from '../config'

export function useOptionalServices(options: { probeControl?: boolean; pollActive?: boolean } = {}) {
  const queryClient = useQueryClient()
  const [actionError, setActionError] = useState<string | null>(null)

  const pollActive = options.pollActive ?? false
  const statusPollMs = pollActive ? 15_000 : 60_000

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
    enabled: SETUP_CONTROL_AVAILABLE && Boolean(options.probeControl),
    staleTime: 10_000,
    retry: false,
  })

  const hostDiagnostics = useQuery({
    queryKey: ['host-diagnostics'],
    queryFn: getHostDiagnostics,
    enabled: SETUP_CONTROL_AVAILABLE && Boolean(options.probeControl),
    staleTime: 10_000,
    refetchInterval: statusPollMs,
    retry: false,
  })

  const hasServiceSnapshot = Boolean(setup.data || diagnostics.data || hostDiagnostics.data)
  const statusLoading = !hasServiceSnapshot && (setup.isLoading || diagnostics.isLoading || hostDiagnostics.isLoading)
  const profile = setup.data?.profile ?? diagnostics.data?.profile ?? hostDiagnostics.data?.profile ?? 'core'
  const services = useMemo(() => {
    const welcome = setup.data?.services
    const diag = diagnostics.data?.services
    return {
      scraper: diag?.scraper === 'ready' || welcome?.scraper === 'ready'
        ? 'ready'
        : welcome?.scraper ?? diag?.scraper ?? 'offline',
      clipper: 'offline' as const,
      pulse: 'offline' as const,
    } as const
  }, [setup.data?.services, diagnostics.data?.services])

  const controlReady = Boolean(control.data?.ok)

  const refreshStatus = async () => {
    setActionError(null)
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['setup-welcome'] }),
      queryClient.invalidateQueries({ queryKey: ['setup-diagnostics'] }),
      SETUP_CONTROL_AVAILABLE && Boolean(options.probeControl)
        ? queryClient.invalidateQueries({ queryKey: ['host-diagnostics'] })
        : Promise.resolve(),
      SETUP_CONTROL_AVAILABLE && Boolean(options.probeControl)
        ? queryClient.invalidateQueries({ queryKey: ['setup-control-health'] })
        : Promise.resolve(),
    ])
  }

  return {
    setup,
    diagnostics,
    control,
    hasServiceSnapshot,
    statusLoading,
    profile,
    services,
    controlReady,
    scraperOffline: services.scraper === 'offline',
    clipperReady: false,
    clipperOffline: true,
    pulseOffline: true,
    starting: null,
    startingServices: new Set(),
    startProgress: null,
    startProgressByService: {},
    actionError,
    setActionError,
    startService: async () => {
      setActionError(
        SETUP_CONTROL_WAKE_ENABLED
          ? 'Optional Analytics and ReplayForge services are not part of Streamclone. Use streampulse-backend / replayforge instead.'
          : 'Optional services are not available from this app.',
      )
      return false
    },
    refreshStatus,
    isStarting: () => false,
  }
}
