import { describe, expect, it, vi } from 'vitest'
import { createHttpClient } from './client'
import { ApiError } from './errors'
import { asRecord, readFiniteNumber } from './validate'

const parseValue = (data: unknown) => readFiniteNumber(asRecord(data), 'value')

const jsonResponse = (status: number, body: unknown, type = 'application/json') =>
  new Response(JSON.stringify(body), { status, headers: { 'Content-Type': type } })

/** A fetch that never settles unless its signal aborts. */
const hangingFetch: typeof fetch = (_input, init) =>
  new Promise((_resolve, reject) => {
    const abort = () => {
      reject(new DOMException('Aborted', 'AbortError'))
    }
    if (init?.signal?.aborted) abort()
    init?.signal?.addEventListener('abort', abort)
  })

async function captureError(promise: Promise<unknown>): Promise<ApiError> {
  try {
    await promise
  } catch (error) {
    if (error instanceof ApiError) return error
    throw error
  }
  throw new Error('expected the request to fail')
}

describe('createHttpClient', () => {
  it('sends JSON to an absolute URL and parses the response', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(jsonResponse(200, { value: 3 }))
    const client = createHttpClient({ fetch: fetchMock })

    await expect(client.post('/v1/things', { a: 1 }, parseValue)).resolves.toBe(3)

    expect(fetchMock).toHaveBeenCalledWith(
      `${window.location.origin}/api/v1/things`,
      expect.objectContaining({
        method: 'POST',
        body: '{"a":1}',
        headers: expect.objectContaining({ 'Content-Type': 'application/json' }),
      }),
    )
  })

  it('sends GET requests without a body', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(jsonResponse(200, { value: 1 }))
    const client = createHttpClient({ baseUrl: 'https://api.example.com/', fetch: fetchMock })

    await client.get('/v1/x', parseValue)

    expect(fetchMock).toHaveBeenCalledWith(
      'https://api.example.com/v1/x',
      expect.objectContaining({ method: 'GET', body: null }),
    )
  })

  it('sends PUT, PATCH and DELETE requests', async () => {
    const fetchMock = vi
      .fn<typeof fetch>()
      .mockImplementation(() => Promise.resolve(jsonResponse(200, { value: 1 })))
    const client = createHttpClient({ fetch: fetchMock })
    const url = `${window.location.origin}/api/v1/things/1`

    await client.put('/v1/things/1', { a: 1 }, parseValue)
    await client.patch('/v1/things/1', { a: 2 }, parseValue)
    await client.delete('/v1/things/1', parseValue)

    expect(fetchMock).toHaveBeenNthCalledWith(
      1,
      url,
      expect.objectContaining({ method: 'PUT', body: '{"a":1}' }),
    )
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      url,
      expect.objectContaining({ method: 'PATCH', body: '{"a":2}' }),
    )
    expect(fetchMock).toHaveBeenNthCalledWith(
      3,
      url,
      expect.objectContaining({ method: 'DELETE', body: null }),
    )
  })

  it('passes an empty body to the parser as undefined', async () => {
    const client = createHttpClient({
      fetch: () => Promise.resolve(new Response(null, { status: 204 })),
    })

    await expect(client.delete('/v1/things/1', (data) => data)).resolves.toBeUndefined()
  })

  it('turns problem details into an http ApiError', async () => {
    const problem = {
      type: 'about:blank',
      title: 'Bad Request',
      status: 400,
      code: 'VALIDATION_FAILED',
      detail: 'The request body is invalid.',
      errors: [{ field: 'name', code: 'REQUIRED', message: 'is required' }],
    }
    const client = createHttpClient({
      fetch: () => Promise.resolve(jsonResponse(400, problem, 'application/problem+json')),
    })

    const error = await captureError(client.get('/x', parseValue))

    expect(error).toMatchObject({ kind: 'http', status: 400, code: 'VALIDATION_FAILED' })
    expect(error.message).toBe('The request body is invalid.')
    expect(error.fieldErrors()).toEqual({ name: 'is required' })
  })

  it('reports error responses without problem details', async () => {
    const client = createHttpClient({
      fetch: () => Promise.resolve(new Response('', { status: 503 })),
    })

    const error = await captureError(client.get('/x', parseValue))

    expect(error).toMatchObject({ kind: 'http', status: 503, code: undefined })
    expect(error.message).toBe('Request failed with status 503.')
  })

  it.each([
    { status: 502, page: '<html><body>Bad Gateway</body></html>' },
    { status: 413, page: '<html><body>Request Entity Too Large</body></html>' },
  ])('reports an HTML $status page from a proxy as an http error', async ({ status, page }) => {
    const client = createHttpClient({
      fetch: () =>
        Promise.resolve(new Response(page, { status, headers: { 'Content-Type': 'text/html' } })),
    })

    const error = await captureError(client.post('/v1/things', {}, parseValue))

    expect(error).toMatchObject({ kind: 'http', status, code: undefined })
    expect(error.message).toBe(`Request failed with status ${status}.`)
  })

  it.each([
    { label: 'a body that is not JSON', response: () => new Response('<html>', { status: 200 }) },
    { label: 'an unexpected shape', response: () => jsonResponse(200, { value: 'many' }) },
  ])('rejects $label as an invalid response', async ({ response }) => {
    const client = createHttpClient({ fetch: () => Promise.resolve(response()) })
    await expect(client.get('/x', parseValue)).rejects.toMatchObject({ kind: 'invalid-response' })
  })

  it('maps network failures', async () => {
    const client = createHttpClient({
      fetch: () => Promise.reject(new TypeError('Failed to fetch')),
    })
    await expect(client.get('/x', parseValue)).rejects.toMatchObject({ kind: 'network' })
  })

  it('times out slow requests', async () => {
    const client = createHttpClient({ fetch: hangingFetch, timeoutMs: 10 })
    await expect(client.get('/x', parseValue)).rejects.toMatchObject({ kind: 'timeout' })
  })

  it('stops when the caller aborts, before or during the request', async () => {
    const client = createHttpClient({ fetch: hangingFetch })

    const controller = new AbortController()
    const pending = client.get('/x', parseValue, { signal: controller.signal })
    controller.abort()
    await expect(pending).rejects.toMatchObject({ kind: 'aborted' })

    await expect(
      client.get('/x', parseValue, { signal: AbortSignal.abort() }),
    ).rejects.toMatchObject({ kind: 'aborted' })
  })
})
