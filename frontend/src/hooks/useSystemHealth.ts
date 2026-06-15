import { useMemo } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { getHostDiagnostics, getMetadataDiagnostics } from '../api'
import { SETUP_CONTROL_AVAILABLE } from '../config'
import { useOptionalServices } from './useOptionalServices'

export function useSystemHealth(options: { probeHost?: boolean; probeControl?: boolean } = {}) {
  const queryClient = useQueryClient()
  const optional = useOptionalServices({ probeControl: options.probeControl })

  const probeHost = Boolean(options.probeHost)

  const host = useQuery({
    queryKey: ['host-diagnostics'],
    queryFn: getHostDiagnostics,
    enabled: SETUP_CONTROL_AVAILABLE && probeHost,
    staleTime: 10_000,
    refetchInterval: false,
    retry: false,
  })

  const metadata = useQuery({
    queryKey: ['metadata-diagnostics'],
    queryFn: getMetadataDiagnostics,
    enabled: probeHost,
    staleTime: 10_000,
    refetchInterval: probeHost ? 20_000 : false,
    retry: false,
  })

  const refreshAll = async () => {
    await Promise.all([
      optional.refreshStatus(),
      SETUP_CONTROL_AVAILABLE && probeHost
        ? queryClient.invalidateQueries({ queryKey: ['host-diagnostics'] })
        : Promise.resolve(),
      probeHost
        ? queryClient.invalidateQueries({ queryKey: ['metadata-diagnostics'] })
        : Promise.resolve(),
    ])
  }

  const diagnosticsBundle = useMemo(() => ({
    generatedAt: new Date().toISOString(),
    profile: optional.profile,
    setupWelcome: optional.setup.data,
    host: host.data,
    metadata: metadata.data,
  }), [host.data, metadata.data, optional.profile, optional.setup.data])

  const coreReady = metadata.data?.healthy ?? Boolean(optional.hasServiceSnapshot && !optional.statusLoading)
  const analyticsReady = optional.services?.scraper === 'ready'
  const clipperReady = optional.services?.clipper === 'ready'
  const installHelperReady = optional.controlReady
  const dockerReady = host.data?.docker === 'running'

  return {
    ...optional,
    host,
    metadata,
    refreshAll,
    diagnosticsBundle,
    coreReady,
    analyticsReady,
    clipperReady,
    installHelperReady,
    dockerReady,
  }
}
