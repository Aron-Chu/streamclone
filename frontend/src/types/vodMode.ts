// Feature: moment-timeline — VOD-mode shared types.
//
// VodStartResponse ALREADY EXISTS in frontend/src/api.ts (extends StartResponse
// with vod_id / offset_seconds / seek_seconds). Re-export it here rather than
// redeclaring so VOD-mode code can import everything from one module.
export type { VodStartResponse } from '../api'

// NEW VOD-mode-specific types below.
export interface VodStartError {
  code: string
  message: string
  retryable: boolean
}
