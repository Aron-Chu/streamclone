import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import App from './App'
import 'streamclone-global-css'

const DEFAULT_QUERY_STALE_TIME_MS = 30_000
const DEFAULT_QUERY_GC_TIME_MS = 5 * 60_000

type RetryableQueryError = {
  status?: number
  retryable?: boolean
}

function shouldRetryQuery(failureCount: number, error: unknown) {
  const queryError = error as RetryableQueryError

  if (queryError.retryable === false) return false
  if (queryError.retryable === true) return failureCount < 2
  if (typeof queryError.status === 'number' && queryError.status >= 400 && queryError.status < 500) return false

  return failureCount < 2
}

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: DEFAULT_QUERY_STALE_TIME_MS,
      gcTime: DEFAULT_QUERY_GC_TIME_MS,
      refetchOnWindowFocus: false,
      retry: shouldRetryQuery,
    },
  },
})

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <App />
      </BrowserRouter>
    </QueryClientProvider>
  </StrictMode>
)
