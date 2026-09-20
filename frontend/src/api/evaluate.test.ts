import { http, HttpResponse } from 'msw'
import { describe, expect, it, vi } from 'vitest'
import { server } from '../test/msw/server'
import { createHttpClient } from './client'
import { ApiError } from './errors'
import { evaluate, isArithmeticCode, parseEvaluateResponse } from './evaluate'

const client = () => createHttpClient({ baseUrl: '/api/v1' })

describe('parseEvaluateResponse', () => {
  it('accepts the contract shape', () => {
    expect(parseEvaluateResponse({ expression: '2*(3+4)', result: '14' })).toEqual({
      expression: '2*(3+4)',
      result: '14',
    })
  })

  it.each([
    { name: 'null', data: null },
    { name: 'an array', data: [] },
    { name: 'a string', data: '14' },
    { name: 'a missing result', data: { expression: '2+2' } },
    { name: 'a numeric result', data: { expression: '2+2', result: 4 } },
    { name: 'a null result', data: { expression: '2+2', result: null } },
    { name: 'a missing expression', data: { result: '4' } },
    { name: 'a non-string expression', data: { expression: 4, result: '4' } },
  ])('rejects $name', ({ data }) => {
    expect(() => parseEvaluateResponse(data)).toThrow(TypeError)
  })
})

describe('isArithmeticCode', () => {
  it.each([
    'DIVISION_BY_ZERO',
    'NEGATIVE_SQUARE_ROOT',
    'INVALID_POWER',
    'EXPONENT_TOO_LARGE',
    'RESULT_TOO_LARGE',
  ])('accepts %s', (code) => {
    expect(isArithmeticCode(code)).toBe(true)
  })

  it.each([undefined, '', 'NEW_CODE', 'division_by_zero'])('rejects %s', (code) => {
    expect(isArithmeticCode(code)).toBe(false)
  })
})

describe('evaluate', () => {
  it('posts the expression to /api/v1/evaluate and returns the parsed response', async () => {
    const bodies: unknown[] = []
    server.use(
      http.post('/api/v1/evaluate', async ({ request }) => {
        bodies.push(await request.json())
        return HttpResponse.json({ expression: '2*(3+4)', result: '14' })
      }),
    )

    await expect(evaluate(client(), '2*(3+4')).resolves.toEqual({
      expression: '2*(3+4)',
      result: '14',
    })
    expect(bodies).toEqual([{ expression: '2*(3+4' }])
  })

  it('uses the default happy-path handler', async () => {
    await expect(evaluate(client(), '2+2')).resolves.toEqual({ expression: '2+2', result: '4' })
  })

  it('maps a validation problem to an http ApiError with the field errors', async () => {
    const error = await evaluate(client(), '2+3)').catch((e: unknown) => e)

    expect(error).toBeInstanceOf(ApiError)
    const apiError = error as ApiError
    expect(apiError.kind).toBe('http')
    expect(apiError.status).toBe(400)
    expect(apiError.code).toBe('VALIDATION_FAILED')
    expect(apiError.problem?.errors).toEqual([
      {
        field: 'expression',
        code: 'UNBALANCED_PARENTHESIS',
        position: 3,
        message: "unbalanced ')' at character 4",
      },
    ])
  })

  it('maps an arithmetic problem to an http ApiError with its code', async () => {
    const error = await evaluate(client(), '1/0').catch((e: unknown) => e)

    expect(error).toMatchObject({ kind: 'http', status: 422, code: 'DIVISION_BY_ZERO' })
  })

  it('rejects a 200 whose result is not a string', async () => {
    server.use(
      http.post('/api/v1/evaluate', () => HttpResponse.json({ expression: '2+2', result: 4 })),
    )

    await expect(evaluate(client(), '2+2')).rejects.toMatchObject({ kind: 'invalid-response' })
  })

  it('forwards the abort signal', async () => {
    const controller = new AbortController()
    controller.abort()

    await expect(evaluate(client(), '2+2', { signal: controller.signal })).rejects.toMatchObject({
      kind: 'aborted',
    })
  })

  it('sends the expression untrimmed so error positions match what the user sees', async () => {
    const post = vi.fn<typeof fetch>().mockResolvedValue(
      new Response(JSON.stringify({ expression: '2 + 2', result: '4' }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    )

    await evaluate(createHttpClient({ baseUrl: '/api/v1', fetch: post }), '  2 + 2  ')

    expect(post).toHaveBeenCalledWith(
      `${window.location.origin}/api/v1/evaluate`,
      expect.objectContaining({ method: 'POST', body: '{"expression":"  2 + 2  "}' }),
    )
  })
})
