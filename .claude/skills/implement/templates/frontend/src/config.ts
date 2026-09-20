import { parsePositiveInt } from './lib/env'

/** Build-time configuration, read once. See .env.example. */
export const config = {
  appName: import.meta.env.VITE_APP_NAME ?? '__APP_NAME__',
  apiBaseUrl: import.meta.env.VITE_API_BASE_URL ?? '/api',
  requestTimeoutMs: parsePositiveInt(import.meta.env.VITE_API_TIMEOUT_MS, 10_000),
} as const
