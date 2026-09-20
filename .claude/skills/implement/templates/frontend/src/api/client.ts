import { ApiError, isProblemDetails } from './errors'

export interface RequestOptions {
  /** Cancels the request, e.g. when a component unmounts or a newer request supersedes it. */
  signal?: AbortSignal
  timeoutMs?: number
}

/** Converts untrusted JSON into a typed value, throwing when the shape is wrong. */
export type Parser<T> = (data: unknown) => T

export interface HttpClient {
  get<T>(path: string, parse: Parser<T>, options?: RequestOptions): Promise<T>
  post<T>(path: string, body: unknown, parse: Parser<T>, options?: RequestOptions): Promise<T>
  put<T>(path: string, body: unknown, parse: Parser<T>, options?: RequestOptions): Promise<T>
  patch<T>(path: string, body: unknown, parse: Parser<T>, options?: RequestOptions): Promise<T>
  /** The parser receives `undefined` when the response has no body (204). */
  delete<T>(path: string, parse: Parser<T>, options?: RequestOptions): Promise<T>
}

export interface HttpClientConfig {
  /** Base path or absolute URL of the API. Defaults to "/api" (same origin, proxied in dev). */
  baseUrl?: string
  timeoutMs?: number
  fetch?: typeof globalThis.fetch
}

const DEFAULT_TIMEOUT_MS = 10_000

export function createHttpClient(config: HttpClientConfig = {}): HttpClient {
  const baseUrl = (config.baseUrl ?? '/api').replace(/\/+$/, '')
  const defaultTimeout = config.timeoutMs ?? DEFAULT_TIMEOUT_MS
  const doFetch = config.fetch ?? ((input, init) => globalThis.fetch(input, init))

  async function request<T>(
    method: string,
    path: string,
    body: unknown,
    parse: Parser<T>,
    options: RequestOptions = {},
  ): Promise<T> {
    if (options.signal?.aborted) throw new ApiError('aborted', 'The request was cancelled.')
    // fetch outside a browser (tests) needs an absolute URL.
    const url = new URL(baseUrl + path, globalThis.location?.origin ?? 'http://localhost')
    const controller = new AbortController()
    let timedOut = false
    const timer = setTimeout(() => {
      timedOut = true
      controller.abort()
    }, options.timeoutMs ?? defaultTimeout)
    const forwardAbort = () => controller.abort()
    options.signal?.addEventListener('abort', forwardAbort, { once: true })

    const headers: Record<string, string> = { Accept: 'application/json' }
    if (body !== undefined) headers['Content-Type'] = 'application/json'

    try {
      const response = await doFetch(url.href, {
        method,
        headers,
        body: body === undefined ? null : JSON.stringify(body),
        signal: controller.signal,
      })
      const payload = await readBody(response)
      if (!response.ok) {
        // A proxy may answer with an HTML page (502, 413…): the status still counts.
        const problem = payload.ok && isProblemDetails(payload.data) ? payload.data : undefined
        const message =
          problem?.detail ?? problem?.title ?? `Request failed with status ${response.status}.`
        throw new ApiError('http', message, { status: response.status, problem })
      }
      if (!payload.ok) {
        throw new ApiError(
          'invalid-response',
          'The server sent a response that is not valid JSON.',
          {
            status: response.status,
            cause: payload.cause,
          },
        )
      }
      try {
        return parse(payload.data)
      } catch (cause) {
        throw new ApiError('invalid-response', 'The server sent an unexpected response.', {
          status: response.status,
          cause,
        })
      }
    } catch (error) {
      if (error instanceof ApiError) throw error
      if (timedOut) {
        throw new ApiError('timeout', 'The server took too long to respond.', { cause: error })
      }
      if (controller.signal.aborted) {
        throw new ApiError('aborted', 'The request was cancelled.', { cause: error })
      }
      throw new ApiError(
        'network',
        'Could not reach the server. Check your connection and try again.',
        {
          cause: error,
        },
      )
    } finally {
      clearTimeout(timer)
      options.signal?.removeEventListener('abort', forwardAbort)
    }
  }

  return {
    get: (path, parse, options) => request('GET', path, undefined, parse, options),
    post: (path, body, parse, options) => request('POST', path, body, parse, options),
    put: (path, body, parse, options) => request('PUT', path, body, parse, options),
    patch: (path, body, parse, options) => request('PATCH', path, body, parse, options),
    delete: (path, parse, options) => request('DELETE', path, undefined, parse, options),
  }
}

type Body = { ok: true; data: unknown } | { ok: false; cause: unknown }

/** Reads the body as JSON without throwing; an empty body is `undefined`. */
async function readBody(response: Response): Promise<Body> {
  const text = await response.text()
  if (text === '') return { ok: true, data: undefined }
  try {
    return { ok: true, data: JSON.parse(text) as unknown }
  } catch (cause) {
    return { ok: false, cause }
  }
}
