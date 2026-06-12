import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  getSetupControlHealth,
  getSetupWelcome,
  startSetupService,
  type SetupWelcome,
} from '../api'

export function useOptionalServices() {
  const queryClient = useQueryClient()
  const [starting, setStarting] = useState<'scraper' | 'clipper' | null>(null)
  const [actionError, setActionError] = useState<string | null>(null)

  const setup = useQuery({
    queryKey: ['setup-welcome'],
    queryFn: getSetupWelcome,
    staleTime: 10_000,
    refetchInterval: 15_000,
  })
  const control = useQuery({
    queryKey: ['setup-control-health'],
    queryFn: getSetupControlHealth,
    staleTime: 10_000,
    retry: false,
  })

  const profile = setup.data?.profile ?? 'core'
  const services = setup.data?.services
  const controlReady = Boolean(control.data?.ok)
  const scraperOffline = services?.scraper === 'offline'
  const clipperOffline = services?.clipper === 'offline'

  const refreshStatus = async () => {
    setActionError(null)
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['setup-welcome'] }),
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
    try {
      await startSetupService(service)
      const maxAttempts = service === 'clipper' ? 45 : 30
      for (let attempt = 0; attempt < maxAttempts; attempt++) {
        await new Promise(resolve => window.setTimeout(resolve, 2000))
        await queryClient.invalidateQueries({ queryKey: ['setup-welcome'] })
        const latest = queryClient.getQueryData<SetupWelcome>(['setup-welcome'])
        if (latest?.services[service] === 'ready') return true
      }
      setActionError(
        service === 'scraper'
          ? 'Analytics is still starting. Check Docker Desktop and retry in a minute.'
          : 'Clip Studio is still starting. Check Docker Desktop and retry in a minute.',
      )
      return false
    } catch (err) {
      setActionError(err instanceof Error ? err.message : `Unable to start ${service}.`)
      return false
    } finally {
      setStarting(null)
    }
  }

  return {
    setup,
    control,
    profile,
    services,
    controlReady,
    scraperOffline,
    clipperOffline,
    starting,
    actionError,
    setActionError,
    startService,
    refreshStatus,
  }
}
