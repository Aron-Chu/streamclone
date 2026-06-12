import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  getSetupControlHealth,
  getSetupStartStatus,
  getSetupWelcome,
  startSetupService,
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
        detail: 'Bringing optional containers online...',
      })
      const maxAttempts = service === 'clipper' ? 45 : 30
      for (let attempt = 0; attempt < maxAttempts; attempt++) {
        await new Promise(resolve => window.setTimeout(resolve, 2000))
        try {
          const status = await getSetupStartStatus(service)
          const pollPercent = Math.min(95, Math.max(18, status.percent))
          setStartProgress({
            service,
            percent: pollPercent,
            phase: status.phase,
            detail: status.detail,
          })
        } catch {
          setStartProgress(prev => prev && prev.service === service
            ? {
                ...prev,
                percent: Math.min(90, prev.percent + 2),
                phase: 'Still starting',
              }
            : prev)
        }
        await queryClient.invalidateQueries({ queryKey: ['setup-welcome'] })
        const latest = queryClient.getQueryData<SetupWelcome>(['setup-welcome'])
        if (latest?.services[service] === 'ready') {
          setStartProgress({
            service,
            percent: 100,
            phase: 'Ready',
            detail: `${START_LABELS[service]} is online`,
          })
          return true
        }
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
      window.setTimeout(() => setStartProgress(null), 2500)
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
    startProgress,
    actionError,
    setActionError,
    startService,
    refreshStatus,
  }
}
