import { act, renderHook } from '@testing-library/react'
import { delay, http, HttpResponse } from 'msw'
import { createElement, type ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ApiProvider } from '../../api/ApiProvider'
import type { HttpClient, RequestOptions } from '../../api/client'
import type { FailureReporter } from '../../lib/report'
import { EVALUATE_URL, problemResponse } from '../../test/msw/handlers'
import { server } from '../../test/msw/server'
import { createAppClient } from '../../test/render'
import { DEBOUNCE_MS, SLOW_MS, useCalculator } from './useCalculator'

const UNAVAILABLE = 'The calculator service is unavailable. Try again.'

/** Resolves each request by hand so response timing is under the test's control. */
interface Deferred {
  expression: string
  resolve: (response: { expression: string; result: string }) => void
}

function deferredHandler(pending: Deferred[]) {
  return http.post(EVALUATE_URL, async ({ request }) => {
    const { expression } = (await request.json()) as { expression: string }
    const response = await new Promise<{ expression: string; result: string }>((resolve) => {
      pending.push({ expression, resolve })
    })
    return HttpResponse.json(response)
  })
}

/** Records the expressions sent and the signal of every request. */
function recordingClient(inner: HttpClient, sent: { expression: string; signal?: AbortSignal }[]) {
  const post: HttpClient['post'] = (path, body, parse, options?: RequestOptions) => {
    const { expression } = body as { expression: string }
    const request = options?.signal ? { expression, signal: options.signal } : { expression }
    sent.push(request)
    return inner.post(path, body, parse, options)
  }
  return { ...inner, post }
}

function setup(client: HttpClient = createAppClient(), report?: FailureReporter) {
  const wrapper = ({ children }: { children: ReactNode }) =>
    createElement(ApiProvider, { client, children })
  return renderHook(() => (report ? useCalculator(report) : useCalculator()), { wrapper })
}

/** The request ID `problemResponse` puts in every problem body. */
const PROBLEM_REQUEST_ID = '4f9c0e7d8a1b2c3d4e5f60718293a4b5'

/** Lets MSW and the client settle without advancing the fake clock. */
async function settle() {
  await act(async () => {
    for (let i = 0; i < 50; i += 1) await Promise.resolve()
    await vi.advanceTimersByTimeAsync(0)
  })
}

async function advance(ms: number) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms)
  })
  await settle()
}

describe('useCalculator', () => {
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('sends exactly one request 150 ms after the last edit and shows the result (FR-9.2)', async () => {
    const sent: { expression: string }[] = []
    const { result } = setup(recordingClient(createAppClient(), sent))

    act(() => {
      result.current.edit('2+2')
    })
    await advance(DEBOUNCE_MS - 1)
    expect(sent).toEqual([])

    await advance(1)
    expect(sent.map((r) => r.expression)).toEqual(['2+2'])
    expect(result.current.state.result).toBe('4')
    expect(result.current.state.phase).toBe('idle')
  })

  it('sends nothing for the intermediate text when the next edit comes within 150 ms (FR-9.2)', async () => {
    const sent: { expression: string }[] = []
    const { result } = setup(recordingClient(createAppClient(), sent))

    act(() => {
      result.current.edit('2+2')
    })
    await advance(100)
    act(() => {
      result.current.edit('2+2*3')
    })
    await advance(DEBOUNCE_MS)

    expect(sent.map((r) => r.expression)).toEqual(['2+2*3'])
    expect(result.current.state.result).toBe('8')
  })

  it.each(['', '   '])('sends no request for %j (A-10)', async (text) => {
    const sent: { expression: string }[] = []
    const { result } = setup(recordingClient(createAppClient(), sent))

    act(() => {
      result.current.edit(text)
    })
    await advance(DEBOUNCE_MS * 2)

    expect(sent).toEqual([])
    expect(result.current.state).toMatchObject({ result: null, message: null, phase: 'idle' })
  })

  it('aborts the request in flight when the input changes and never applies its response (FR-9.3)', async () => {
    const pending: Deferred[] = []
    server.use(deferredHandler(pending))
    const sent: { expression: string; signal?: AbortSignal }[] = []
    const { result } = setup(recordingClient(createAppClient(), sent))

    act(() => {
      result.current.edit('2+2')
    })
    await advance(DEBOUNCE_MS)
    expect(pending.map((p) => p.expression)).toEqual(['2+2'])

    act(() => {
      result.current.edit('2+2*3')
    })
    await settle()
    expect(sent[0]?.signal?.aborted).toBe(true)

    pending[0]?.resolve({ expression: '2+2', result: '4' })
    await settle()
    expect(result.current.state.result).toBeNull()

    await advance(DEBOUNCE_MS)
    expect(pending.map((p) => p.expression)).toEqual(['2+2', '2+2*3'])
    expect(sent[1]?.signal?.aborted).toBe(false)
    pending[1]?.resolve({ expression: '2+2*3', result: '8' })
    await settle()
    expect(result.current.state.result).toBe('8')
  })

  it('keeps the previous result while a response arrives within 300 ms (FR-9.6)', async () => {
    const pending: Deferred[] = []
    server.use(deferredHandler(pending))
    const { result } = setup()

    act(() => {
      result.current.edit('2+2')
    })
    await advance(DEBOUNCE_MS)
    pending[0]?.resolve({ expression: '2+2', result: '4' })
    await settle()
    expect(result.current.state.result).toBe('4')

    act(() => {
      result.current.edit('2+2*3')
    })
    await advance(DEBOUNCE_MS)
    await advance(200)
    expect(result.current.state).toMatchObject({ result: '4', calculating: false })

    pending[1]?.resolve({ expression: '2+2*3', result: '8' })
    await settle()
    expect(result.current.state).toMatchObject({ result: '8', calculating: false })

    await advance(SLOW_MS)
    expect(result.current.state.calculating).toBe(false)
  })

  it('replaces the result with "Calculating…" 300 ms after the request until it answers (FR-9.6)', async () => {
    const pending: Deferred[] = []
    server.use(deferredHandler(pending))
    const { result } = setup()

    act(() => {
      result.current.edit('2+2')
    })
    await advance(DEBOUNCE_MS)
    pending[0]?.resolve({ expression: '2+2', result: '4' })
    await settle()

    act(() => {
      result.current.edit('2+2*3')
    })
    await advance(DEBOUNCE_MS)
    await advance(SLOW_MS - 1)
    expect(result.current.state).toMatchObject({ result: '4', calculating: false })

    await advance(1)
    expect(result.current.state).toMatchObject({ result: null, calculating: true })

    await advance(100)
    pending[1]?.resolve({ expression: '2+2*3', result: '8' })
    await settle()
    expect(result.current.state).toMatchObject({ result: '8', calculating: false })
  })

  it('does not start the slow timer before the debounce has elapsed', async () => {
    const pending: Deferred[] = []
    server.use(deferredHandler(pending))
    const { result } = setup()

    act(() => {
      result.current.edit('2+2')
    })
    await advance(DEBOUNCE_MS + SLOW_MS - 1)
    expect(result.current.state.calculating).toBe(false)
    await advance(1)
    expect(result.current.state.calculating).toBe(true)
  })

  it('sends no live request after a commit until the input changes (FR-9.5)', async () => {
    const sent: { expression: string }[] = []
    const { result } = setup(recordingClient(createAppClient(), sent))

    act(() => {
      result.current.edit('2+2')
    })
    await advance(DEBOUNCE_MS)
    act(() => {
      result.current.commit()
    })
    await advance(0)
    expect(sent.map((r) => r.expression)).toEqual(['2+2', '2+2'])
    expect(result.current.state).toMatchObject({
      expression: '4',
      phase: 'committed',
      history: [{ expression: '2+2', result: '4' }],
    })

    await advance(DEBOUNCE_MS * 10)
    expect(sent).toHaveLength(2)

    act(() => {
      result.current.edit('4*3')
    })
    await advance(DEBOUNCE_MS)
    expect(sent.map((r) => r.expression)).toEqual(['2+2', '2+2', '4*3'])
    expect(result.current.state.result).toBe('12')
  })

  it('cancels the pending debounce, aborts the live request and sends its own on commit (FR-11.4)', async () => {
    const pending: Deferred[] = []
    server.use(deferredHandler(pending))
    const sent: { expression: string; signal?: AbortSignal }[] = []
    const { result } = setup(recordingClient(createAppClient(), sent))

    act(() => {
      result.current.edit('2+2')
    })
    await advance(DEBOUNCE_MS)
    expect(pending).toHaveLength(1)
    act(() => {
      result.current.edit('2+2*3')
    })
    await advance(50)
    act(() => {
      result.current.commit()
    })
    await advance(0)

    expect(sent[0]?.signal?.aborted).toBe(true)
    expect(sent.map((r) => r.expression)).toEqual(['2+2', '2+2*3'])
    expect(result.current.state.phase).toBe('committing')

    await advance(DEBOUNCE_MS)
    expect(sent).toHaveLength(2)

    pending[1]?.resolve({ expression: '2+2*3', result: '8' })
    await settle()
    expect(result.current.state).toMatchObject({ expression: '8', phase: 'committed' })
  })

  it('ignores a second commit while one is in flight (FR-11.4)', async () => {
    const pending: Deferred[] = []
    server.use(deferredHandler(pending))
    const sent: { expression: string }[] = []
    const { result } = setup(recordingClient(createAppClient(), sent))

    act(() => {
      result.current.edit('2+2')
      result.current.commit()
    })
    await advance(0)
    act(() => {
      result.current.commit()
    })
    await advance(0)

    expect(sent).toHaveLength(1)
    pending[0]?.resolve({ expression: '2+2', result: '4' })
    await settle()
    expect(result.current.state.history).toHaveLength(1)
  })

  it('aborts a commit when the input is edited during it and follows the live path (D-4)', async () => {
    const pending: Deferred[] = []
    server.use(deferredHandler(pending))
    const sent: { expression: string; signal?: AbortSignal }[] = []
    const { result } = setup(recordingClient(createAppClient(), sent))

    act(() => {
      result.current.edit('2+2')
      result.current.commit()
    })
    await advance(0)
    act(() => {
      result.current.edit('2+2*3')
    })
    await settle()

    expect(sent[0]?.signal?.aborted).toBe(true)
    expect(result.current.state.phase).toBe('live')
    pending[0]?.resolve({ expression: '2+2', result: '4' })
    await settle()
    expect(result.current.state).toMatchObject({ expression: '2+2*3', history: [] })

    await advance(DEBOUNCE_MS)
    pending[1]?.resolve({ expression: '2+2*3', result: '8' })
    await settle()
    expect(result.current.state).toMatchObject({ result: '8', phase: 'idle' })
  })

  it('shows the commit error as an alert and keeps the input (FR-11.2)', async () => {
    const { result } = setup()

    act(() => {
      result.current.edit('1/0')
      result.current.commit()
    })
    await advance(0)

    expect(result.current.state).toMatchObject({
      expression: '1/0',
      invalid: true,
      message: { kind: 'alert', text: 'Cannot divide by zero.' },
    })
  })

  it('shows the network message when no response arrives within 10 s (FR-13.4)', async () => {
    server.use(
      http.post(EVALUATE_URL, async () => {
        await delay('infinite')
        return HttpResponse.json({ expression: '', result: '' })
      }),
    )
    const { result } = setup()

    act(() => {
      result.current.edit('2+2')
    })
    await advance(DEBOUNCE_MS)
    await advance(9_999)
    expect(result.current.state.message).toBeNull()

    await advance(1)
    expect(result.current.state).toMatchObject({
      message: { kind: 'status', text: UNAVAILABLE },
      invalid: false,
      calculating: false,
    })
  })

  it('shows a committed 5xx as an alert without marking the input invalid', async () => {
    server.use(http.post(EVALUATE_URL, () => problemResponse(500, 'INTERNAL_ERROR')))
    const { result } = setup()

    act(() => {
      result.current.edit('2+2')
      result.current.commit()
    })
    await advance(0)

    expect(result.current.state).toMatchObject({
      message: { kind: 'alert', text: UNAVAILABLE },
      invalid: false,
      expression: '2+2',
    })
  })

  it('clears everything and aborts the request in flight (FR-12.2)', async () => {
    const pending: Deferred[] = []
    server.use(deferredHandler(pending))
    const sent: { expression: string; signal?: AbortSignal }[] = []
    const { result } = setup(recordingClient(createAppClient(), sent))

    act(() => {
      result.current.edit('2+2')
    })
    await advance(DEBOUNCE_MS)
    act(() => {
      result.current.clear()
    })
    await settle()

    expect(sent[0]?.signal?.aborted).toBe(true)
    expect(result.current.state).toMatchObject({ expression: '', result: null, phase: 'idle' })
    await advance(DEBOUNCE_MS * 2)
    expect(sent).toHaveLength(1)
  })

  it('cancels a pending debounce on clear', async () => {
    const sent: { expression: string }[] = []
    const { result } = setup(recordingClient(createAppClient(), sent))

    act(() => {
      result.current.edit('2+2')
    })
    await advance(50)
    act(() => {
      result.current.clear()
    })
    await advance(DEBOUNCE_MS * 2)

    expect(sent).toEqual([])
  })

  it('aborts the request in flight on unmount', async () => {
    const pending: Deferred[] = []
    server.use(deferredHandler(pending))
    const sent: { expression: string; signal?: AbortSignal }[] = []
    const { result, unmount } = setup(recordingClient(createAppClient(), sent))

    act(() => {
      result.current.edit('2+2')
    })
    await advance(DEBOUNCE_MS)
    unmount()

    expect(sent[0]?.signal?.aborted).toBe(true)
  })

  it('reloads a history entry through the debounced live path (FR-14.2)', async () => {
    const sent: { expression: string }[] = []
    const { result } = setup(recordingClient(createAppClient(), sent))

    act(() => {
      result.current.edit('2+2')
      result.current.commit()
    })
    await advance(0)
    act(() => {
      result.current.edit('3*3')
      result.current.commit()
    })
    await advance(0)
    expect(result.current.state.history.map((e) => e.expression)).toEqual(['3*3', '2+2'])

    act(() => {
      result.current.activateHistory(1)
    })
    expect(result.current.state).toMatchObject({ expression: '2+2', phase: 'live', result: null })
    await advance(DEBOUNCE_MS - 1)
    expect(sent).toHaveLength(2)
    await advance(1)
    expect(sent.map((r) => r.expression)).toEqual(['2+2', '3*3', '2+2'])
    expect(result.current.state.result).toBe('4')
  })

  describe('failure reporting for support', () => {
    it('reports a 503 with its kind, status, code and request ID', async () => {
      server.use(http.post(EVALUATE_URL, () => problemResponse(503, 'TIMEOUT')))
      const report = vi.fn<FailureReporter>()
      const { result } = setup(createAppClient(), report)

      act(() => {
        result.current.edit('2+2')
      })
      await advance(DEBOUNCE_MS)

      expect(result.current.state.message).toEqual({ kind: 'status', text: UNAVAILABLE })
      expect(report).toHaveBeenCalledExactlyOnceWith('evaluate request failed', {
        kind: 'http',
        status: 503,
        code: 'TIMEOUT',
        requestId: PROBLEM_REQUEST_ID,
      })
    })

    it('reports a network failure without status, code or request ID', async () => {
      server.use(http.post(EVALUATE_URL, () => HttpResponse.error()))
      const report = vi.fn<FailureReporter>()
      const { result } = setup(createAppClient(), report)

      act(() => {
        result.current.edit('2+2')
        result.current.commit()
      })
      await advance(0)

      expect(result.current.state.message).toEqual({ kind: 'alert', text: UNAVAILABLE })
      expect(report).toHaveBeenCalledExactlyOnceWith('evaluate request failed', {
        kind: 'network',
        status: undefined,
        code: undefined,
        requestId: undefined,
      })
    })

    it('reports a response the client cannot parse', async () => {
      server.use(http.post(EVALUATE_URL, () => HttpResponse.json({ unexpected: true })))
      const report = vi.fn<FailureReporter>()
      const { result } = setup(createAppClient(), report)

      act(() => {
        result.current.edit('2+2')
      })
      await advance(DEBOUNCE_MS)

      expect(report).toHaveBeenCalledExactlyOnceWith('evaluate request failed', {
        kind: 'invalid-response',
        status: 200,
        code: undefined,
        requestId: undefined,
      })
    })

    it.each([
      { name: 'a 400 validation error', expression: '2+3)' },
      { name: 'a 422 arithmetic error', expression: '1/0' },
    ])('does not report $name', async ({ expression }) => {
      const report = vi.fn<FailureReporter>()
      const { result } = setup(createAppClient(), report)

      act(() => {
        result.current.edit(expression)
      })
      await advance(DEBOUNCE_MS)
      act(() => {
        result.current.commit()
      })
      await advance(0)

      expect(result.current.state.message?.kind).toBe('alert')
      expect(report).not.toHaveBeenCalled()
    })

    it('does not report a request that was aborted by a newer edit', async () => {
      const pending: Deferred[] = []
      server.use(deferredHandler(pending))
      const report = vi.fn<FailureReporter>()
      const { result } = setup(createAppClient(), report)

      act(() => {
        result.current.edit('2+2')
      })
      await advance(DEBOUNCE_MS)
      expect(pending).toHaveLength(1)
      act(() => {
        result.current.clear()
      })
      await settle()
      await advance(DEBOUNCE_MS)

      expect(report).not.toHaveBeenCalled()
    })

    it('warns on the console by default', async () => {
      const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
      server.use(http.post(EVALUATE_URL, () => problemResponse(500, 'INTERNAL_ERROR')))
      const { result } = setup()

      act(() => {
        result.current.edit('2+2')
      })
      await advance(DEBOUNCE_MS)

      expect(warn).toHaveBeenCalledExactlyOnceWith('evaluate request failed', {
        kind: 'http',
        status: 500,
        code: 'INTERNAL_ERROR',
        requestId: PROBLEM_REQUEST_ID,
      })
    })
  })

  it('appends keypad tokens and removes characters through the reducer', async () => {
    const { result } = setup()

    act(() => {
      result.current.pressKey('sqrt')
      result.current.pressKey('1')
      result.current.pressKey('6')
      result.current.pressKey(')')
      result.current.pressKey(')')
      result.current.backspace()
    })
    await advance(DEBOUNCE_MS)

    expect(result.current.state).toMatchObject({ expression: 'sqrt(16)', result: '4' })
  })

  it('appends typed characters through the reducer and evaluates them (decision 38)', async () => {
    const { result } = setup()

    act(() => {
      result.current.typeCharacter('2')
      result.current.typeCharacter('+')
      result.current.typeCharacter('2')
    })
    await advance(DEBOUNCE_MS)

    expect(result.current.state).toMatchObject({ expression: '2+2', result: '4' })
  })
})
