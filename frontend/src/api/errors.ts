/** RFC 9457 problem details, as returned by the backend for every error response. */
export interface ProblemDetails {
  type: string
  title: string
  status: number
  detail?: string
  instance?: string
  /** Stable machine-readable code, e.g. VALIDATION_FAILED. Branch on this, never on text. */
  code: string
  requestId?: string
  errors?: FieldError[]
}

export interface FieldError {
  field: string
  code: string
  message: string
}

export type ApiErrorKind = 'network' | 'timeout' | 'aborted' | 'http' | 'invalid-response'

interface ApiErrorOptions {
  status?: number | undefined
  problem?: ProblemDetails | undefined
  cause?: unknown
}

/** The only error type the API layer throws. UI code switches on `kind` and `code`. */
export class ApiError extends Error {
  readonly kind: ApiErrorKind
  readonly status: number | undefined
  readonly problem: ProblemDetails | undefined

  constructor(kind: ApiErrorKind, message: string, options: ApiErrorOptions = {}) {
    super(message, { cause: options.cause })
    this.name = 'ApiError'
    this.kind = kind
    this.status = options.status
    this.problem = options.problem
  }

  get code(): string | undefined {
    return this.problem?.code
  }

  /** Server-side validation messages keyed by field name. */
  fieldErrors(): Record<string, string> {
    return Object.fromEntries((this.problem?.errors ?? []).map((e) => [e.field, e.message]))
  }
}

export function isProblemDetails(value: unknown): value is ProblemDetails {
  if (typeof value !== 'object' || value === null) return false
  const v = value as Record<string, unknown>
  return typeof v.status === 'number' && typeof v.title === 'string' && typeof v.code === 'string'
}

export function toApiError(error: unknown): ApiError {
  if (error instanceof ApiError) return error
  return new ApiError('network', 'Something went wrong. Please try again.', { cause: error })
}
