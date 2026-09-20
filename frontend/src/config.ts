import { apiBaseUrl, parsePositiveInt } from './lib/env'

/** Build-time configuration, read once. See .env.example. */
export const config = {
  appName: import.meta.env.VITE_APP_NAME ?? 'Calculator',
  /** VITE_API_BASE_URL (empty = same origin) with `/api/v1` appended. */
  apiBaseUrl: apiBaseUrl(import.meta.env.VITE_API_BASE_URL),
  requestTimeoutMs: parsePositiveInt(import.meta.env.VITE_API_TIMEOUT_MS, 10_000),
} as const
