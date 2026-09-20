import type { HttpClient, RequestOptions } from './client'
import { asRecord, readString } from './validate'

/** Request body of POST /api/v1/evaluate. */
export interface EvaluateRequest {
  expression: string
}

/** 200 response of POST /api/v1/evaluate: the normalized expression and its exact result. */
export interface EvaluateResponse {
  expression: string
  /** Canonical decimal string (e.g. "14", "-0.5"); never a JSON number, so no precision is lost. */
  result: string
}

/** `errors[].code` values a 400 VALIDATION_FAILED response can carry for `expression`. */
export const VALIDATION_CODES = [
  'REQUIRED',
  'EMPTY',
  'TOO_LONG',
  'TOO_DEEP',
  'INVALID_CHARACTER',
  'INVALID_NUMBER',
  'UNEXPECTED_TOKEN',
  'UNBALANCED_PARENTHESIS',
  'UNKNOWN_FUNCTION',
] as const
export type ValidationCode = (typeof VALIDATION_CODES)[number]

/** Problem `code` values of a 422 response. */
export const ARITHMETIC_CODES = [
  'DIVISION_BY_ZERO',
  'NEGATIVE_SQUARE_ROOT',
  'INVALID_POWER',
  'EXPONENT_TOO_LARGE',
  'RESULT_TOO_LARGE',
] as const
export type ArithmeticCode = (typeof ARITHMETIC_CODES)[number]

export function isArithmeticCode(value: string | undefined): value is ArithmeticCode {
  return value !== undefined && (ARITHMETIC_CODES as readonly string[]).includes(value)
}

export function parseEvaluateResponse(data: unknown): EvaluateResponse {
  const record = asRecord(data, 'evaluate response')
  return {
    expression: readString(record, 'expression'),
    result: readString(record, 'result'),
  }
}

export function evaluate(
  client: HttpClient,
  expression: string,
  options?: RequestOptions,
): Promise<EvaluateResponse> {
  const body: EvaluateRequest = { expression }
  return client.post('/evaluate', body, parseEvaluateResponse, options)
}
