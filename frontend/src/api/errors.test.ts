import { describe, expect, it } from 'vitest'
import { ApiError, isProblemDetails, toApiError } from './errors'

describe('isProblemDetails', () => {
  it.each([
    {
      label: 'a problem document',
      value: { type: 'about:blank', title: 'Bad Request', status: 400, code: 'X' },
      expected: true,
    },
    {
      label: 'an object without a code',
      value: { title: 'Bad Request', status: 400 },
      expected: false,
    },
    { label: 'null', value: null, expected: false },
    { label: 'a string', value: 'error', expected: false },
  ])('returns $expected for $label', ({ value, expected }) => {
    expect(isProblemDetails(value)).toBe(expected)
  })
})

describe('toApiError', () => {
  it('returns an ApiError unchanged', () => {
    const error = new ApiError('timeout', 'slow')
    expect(toApiError(error)).toBe(error)
  })

  it('wraps anything else as a network error that keeps the cause', () => {
    const cause = new TypeError('boom')
    expect(toApiError(cause)).toMatchObject({ kind: 'network', cause })
  })
})

describe('ApiError', () => {
  it('exposes the problem code and the field errors', () => {
    const error = new ApiError('http', 'invalid', {
      status: 400,
      problem: {
        type: 'about:blank',
        title: 'Bad Request',
        status: 400,
        code: 'VALIDATION_FAILED',
        errors: [{ field: 'name', code: 'REQUIRED', message: 'is required' }],
      },
    })

    expect(error.code).toBe('VALIDATION_FAILED')
    expect(error.fieldErrors()).toEqual({ name: 'is required' })
    expect(new ApiError('network', 'down').fieldErrors()).toEqual({})
  })
})
