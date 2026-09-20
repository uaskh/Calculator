import { http, HttpResponse, type RequestHandler, type StrictResponse } from 'msw'
import type { ProblemDetails } from '../../api/errors'
import type { EvaluateResponse } from '../../api/evaluate'

type EvaluateResult = StrictResponse<EvaluateResponse | ProblemDetails>

export const EVALUATE_URL = '/api/v1/evaluate'

const TITLES: Record<number, string> = {
  400: 'Bad Request',
  404: 'Not Found',
  413: 'Content Too Large',
  415: 'Unsupported Media Type',
  422: 'Unprocessable Content',
  500: 'Internal Server Error',
  503: 'Service Unavailable',
}

/** An RFC 9457 problem response with the given status and code. */
export function problemResponse(
  status: number,
  code: string,
  extra: Partial<ProblemDetails> = {},
): StrictResponse<ProblemDetails> {
  const body: ProblemDetails = {
    type: 'about:blank',
    title: TITLES[status] ?? 'Error',
    status,
    instance: EVALUATE_URL,
    code,
    requestId: '4f9c0e7d8a1b2c3d4e5f60718293a4b5',
    ...extra,
  }
  return HttpResponse.json(body, {
    status,
    headers: { 'Content-Type': 'application/problem+json' },
  })
}

/** A 400 VALIDATION_FAILED problem with the single field error the backend returns. */
export function validationFailed(
  code: string,
  message: string,
  position?: number,
): StrictResponse<ProblemDetails> {
  return problemResponse(400, 'VALIDATION_FAILED', {
    detail: message,
    errors: [
      position === undefined
        ? { field: 'expression', code, message }
        : { field: 'expression', code, position, message },
    ],
  })
}

/** A 422 arithmetic problem. */
export function arithmeticFailed(code: string, detail: string): StrictResponse<ProblemDetails> {
  return problemResponse(422, code, { detail })
}

/**
 * A tiny stand-in for the backend: known expressions get their real result, a few known
 * failures get their real problem, and anything else echoes the trimmed expression as its
 * result. Tests override per case with server.use(...).
 */
const KNOWN_RESULTS: Record<string, { expression?: string; result: string }> = {
  '2+2': { result: '4' },
  '2+2*3': { result: '8' },
  '2*(3+4': { expression: '2*(3+4)', result: '14' },
  '2*(3+4)': { result: '14' },
  '4': { result: '4' },
  '4*3': { result: '12' },
  '3*3': { result: '9' },
  '2-7': { result: '-5' },
  '(-5)^2': { result: '25' },
  '7*(2+1)': { result: '21' },
  'sqrt(16)': { result: '4' },
  '12+3': { result: '15' },
}

const EMPTY_INPUTS = new Set(['', '(', '-', 'sqrt(', '+'])

export function fakeEvaluate(raw: string): EvaluateResult {
  const expression = raw.trim()
  if (EMPTY_INPUTS.has(expression)) return validationFailed('EMPTY', 'expression is empty')
  if (expression === '1/0') return arithmeticFailed('DIVISION_BY_ZERO', 'Cannot divide by zero.')
  if (expression === '2+3)') {
    return validationFailed('UNBALANCED_PARENTHESIS', "unbalanced ')' at character 4", 3)
  }
  const known = KNOWN_RESULTS[expression]
  return HttpResponse.json({
    expression: known?.expression ?? expression,
    result: known?.result ?? expression,
  })
}

function readExpression(body: unknown): string | undefined {
  if (typeof body !== 'object' || body === null || !('expression' in body)) return undefined
  const { expression } = body
  return typeof expression === 'string' ? expression : undefined
}

/**
 * Default happy-path handlers shared by all tests, one per API operation. Tests override
 * them with server.use(...) to simulate errors, slow responses or unusual payloads.
 */
export const handlers: RequestHandler[] = [
  http.post(EVALUATE_URL, async ({ request }): Promise<EvaluateResult> => {
    const body: unknown = await request.json()
    const expression = readExpression(body)
    if (expression === undefined) {
      return problemResponse(400, 'MALFORMED_REQUEST', { detail: 'The request body is invalid.' })
    }
    return fakeEvaluate(expression)
  }),
]
