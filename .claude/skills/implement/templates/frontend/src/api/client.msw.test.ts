// Canary for the test environment: the real client, jsdom, MSW and AbortSignal together.
// If this fails with "Expected signal to be an instance of AbortSignal", switch the Vitest
// environment to 'happy-dom' or align the Vitest/jsdom versions (see the scaffold guide).
import { delay, http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { server } from '../test/msw/server'
import { createHttpClient } from './client'
import { asRecord, readFiniteNumber } from './validate'

const parseValue = (data: unknown) => readFiniteNumber(asRecord(data), 'value')

describe('HTTP client with MSW', () => {
  it('round-trips a request through the mocked network', async () => {
    server.use(
      http.post('/api/v1/echo', async ({ request }) => {
        const body = asRecord(await request.json())
        return HttpResponse.json({ value: readFiniteNumber(body, 'value') * 2 })
      }),
    )

    await expect(createHttpClient().post('/v1/echo', { value: 21 }, parseValue)).resolves.toBe(42)
  })

  it('cancels an in-flight request', async () => {
    server.use(
      http.get('/api/v1/slow', async () => {
        await delay('infinite')
        return HttpResponse.json({ value: 1 })
      }),
    )
    const controller = new AbortController()

    const pending = createHttpClient().get('/v1/slow', parseValue, { signal: controller.signal })
    controller.abort()

    await expect(pending).rejects.toMatchObject({ kind: 'aborted' })
  })
})
