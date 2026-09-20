// Small runtime guards for API responses; they throw TypeError on unexpected shapes.

export function asRecord(value: unknown, what = 'response'): Record<string, unknown> {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) {
    throw new TypeError(`${what} is not an object`)
  }
  return value as Record<string, unknown>
}

export function readString(record: Record<string, unknown>, key: string): string {
  const value = record[key]
  if (typeof value !== 'string') throw new TypeError(`${key} is not a string`)
  return value
}

export function readFiniteNumber(record: Record<string, unknown>, key: string): number {
  const value = record[key]
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    throw new TypeError(`${key} is not a finite number`)
  }
  return value
}
