import { create } from 'zustand'

export interface PlayheadState {
  streamId: string | null
  offsetSeconds: number
  isPlaying: boolean
  vodId: string | null
  setPlayhead: (streamId: string, offset: number, vodId?: string) => void
  setPlaying: (playing: boolean) => void
  reset: () => void
}

const initialState = {
  streamId: null,
  offsetSeconds: 0,
  isPlaying: false,
  vodId: null,
} satisfies Pick<PlayheadState, 'streamId' | 'offsetSeconds' | 'isPlaying' | 'vodId'>

export const usePlayheadStore = create<PlayheadState>(set => ({
  ...initialState,
  setPlayhead: (streamId, offset, vodId) => set(state => ({
    streamId,
    offsetSeconds: Number.isFinite(offset) ? Math.max(0, offset) : 0,
    vodId: vodId ?? (streamId === state.streamId ? state.vodId : null),
  })),
  setPlaying: playing => set({ isPlaying: playing }),
  reset: () => set({ ...initialState }),
}))
