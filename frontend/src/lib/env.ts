/** Parses a positive integer setting, falling back when it is missing or invalid. */
export function parsePositiveInt(raw: string | undefined, fallback: number): number {
  if (raw === undefined || raw.trim() === '') return fallback
  const value = Number(raw)
  return Number.isSafeInteger(value) && value > 0 ? value : fallback
}

/** Versioned base path of the API, appended to the configured origin. */
export const API_BASE_PATH = '/api/v1'

/**
 * Resolves where the API lives: the configured origin (empty means same origin, proxied in
 * development and by nginx in containers) with the versioned base path appended.
 */
export function apiBaseUrl(origin: string | undefined): string {
  return `${(origin ?? '').trim().replace(/\/+$/, '')}${API_BASE_PATH}`
}
