import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

const usePollingWatch = Boolean(
  (globalThis as typeof globalThis & {
    process?: {
      env?: Record<string, string | undefined>
    }
  }).process?.env?.WSL_DISTRO_NAME,
)

export default defineConfig({
  plugins: [react()],
  server: {
    watch: usePollingWatch
      ? {
          usePolling: true,
          interval: 150,
        }
      : undefined,
    proxy: {
      '/live': {
        target: 'http://localhost:8888',
        changeOrigin: true,
      },
      '/emotes': {
        target: 'http://localhost:9000',
        changeOrigin: true,
      },
      '/v1/auth': {
        target: 'http://localhost:8083',
        changeOrigin: true,
      },
      '/v1/me': {
        target: 'http://localhost:8083',
        changeOrigin: true,
      },
      '/v1/logout': {
        target: 'http://localhost:8083',
        changeOrigin: true,
      },
      '/v1/followed': {
        target: 'http://localhost:8081',
        changeOrigin: true,
      },
      '/v1/ws': {
        target: 'ws://localhost:8083',
        ws: true,
        changeOrigin: true,
      },
      '/v1/streams': {
        target: 'http://localhost:8081',
        changeOrigin: true,
      },
      '^/v1/stream(/|$)': {
        target: 'http://localhost:8082',
        changeOrigin: true,
      },
      '^/v1/channels/.*/emotes': {
        target: 'http://localhost:8084',
        changeOrigin: true,
      },
      '/v1/analytics': {
        target: 'http://localhost:8086',
        changeOrigin: true,
      },
      '/v1/channels': {
        target: 'http://localhost:8081',
        changeOrigin: true,
      },
      '/v1/emotes': {
        target: 'http://localhost:8084',
        changeOrigin: true,
      },
      '/v1/sets': {
        target: 'http://localhost:8084',
        changeOrigin: true,
      },
      '/v1/seed': {
        target: 'http://localhost:8084',
        changeOrigin: true,
      },
      '/v1': {
        target: 'http://localhost:8081',
        changeOrigin: true,
      },
    },
  },
  build: {
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (id.includes('/components/Analytics.tsx') || id.includes('/components/analytics/')) return 'analytics'
          if (id.includes('/components/Channel.tsx') || id.includes('/components/channel/')) return 'channel'
          if (!id.includes('node_modules')) return undefined
          if (id.includes('/react/') || id.includes('/react-dom/')) return 'react'
          if (id.includes('/react-router-dom/') || id.includes('/@tanstack/react-query/')) return 'app-core'
          if (id.includes('/zustand/') || id.includes('/@tanstack/react-virtual/')) return 'chat'
          if (id.includes('/hls.js/')) return 'hls'
          return undefined
        },
      },
    },
  },
})
