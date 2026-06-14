import { useMemo } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { getHostDiagnostics, getMetadataDiagnostics } from '../api'
import { SETUP_CONTROL_AVAILABLE } from '../config'
import { useOptionalServices } from './useOptionalServices'

export function useSystemHealth() {
  const queryClient = useQueryClient()
  const optional = useOptionalServices()

  const host = useQuery({
    queryKey: ['host-diagnostics'],
    queryFn: getHostDiagnostics,
    enabled: SETUP_CONTROL_AVAILABLE,
    staleTime: 10_000,
    refetchInterval: 20_000,
    retry: false,
  })

  const metadata = useQuery({
    queryKey: ['metadata-diagnostics'],
    queryFn: getMetadataDiagnostics,
    staleTime: 10_000,
    refetchInterval: 20_000,
    retry: false,
  })

  const refreshAll = async () => {
    await Promise.all([
      optional.refreshStatus(),
      SETUP_CONTROL_AVAILABLE
        ? queryClient.invalidateQueries({ queryKey: ['host-diagnostics'] })
        : Promise.resolve(),
      queryClient.invalidateQueries({ queryKey: ['metadata-diagnostics'] }),
    ])
  }

  const diagnosticsBundle = useMemo(() => ({
    generatedAt: new Date().toISOString(),
    profile: optional.profile,
    setupWelcome: optional.setup.data,
    host: host.data,
    metadata: metadata.data,
  }), [host.data, metadata.data, optional.profile, optional.setup.data])

  const coreReady = metadata.data?.healthy ?? Boolean(optional.setup.data && !optional.setup.isLoading)
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
