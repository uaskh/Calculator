import { describe, expect, it } from 'vitest'
import { ApiError } from '../../api/errors'
import {
  calculatorReducer,
  historyLabel,
  initialState,
  insertKey,
  isEmptyError,
  KEYPAD_KEYS,
  MAX_EXPRESSION_LENGTH,
  MAX_HISTORY,
  messageFor,
  removeLastCharacter,
  wrapResult,
  type CalculatorEvent,
  type CalculatorState,
} from './model'

const httpError = (status: number, code: string, extra: Record<string, unknown> = {}) =>
  new ApiError('http', 'failed', {
    status,
    problem: { type: 'about:blank', title: 'Error', status, code, ...extra },
  })

const validation = (code: string, message: string, position?: number) =>
  httpError(400, 'VALIDATION_FAILED', {
    errors: [
      position === undefined
        ? { field: 'expression', code, message }
        : { field: 'expression', code, position, message },
    ],
  })

const emptyError = validation('EMPTY', 'expression is empty')
const divisionByZero = httpError(422, 'DIVISION_BY_ZERO')
const networkError = new ApiError('network', 'down')

const UNAVAILABLE = 'The calculator service is unavailable. Try again.'
const GENERIC = 'Something went wrong. Try again.'

function reduce(events: CalculatorEvent[], from: CalculatorState = initialState): CalculatorState {
  return events.reduce(calculatorReducer, from)
}

/** A state after typing `text` and receiving a successful live response. */
function afterLive(text: string, response = { expression: text, result: text }): CalculatorState {
  return reduce([
    { type: 'edited', text },
    { type: 'requestStarted' },
    { type: 'liveSucceeded', response },
  ])
}

describe('insertKey', () => {
  it.each([
    { expression: '', key: '7', expected: '7' },
    { expression: '7', key: '*', expected: '7*' },
    { expression: '7*', key: '(', expected: '7*(' },
    { expression: '2', key: '.', expected: '2.' },
    { expression: '', key: 'sqrt', expected: 'sqrt(' },
    { expression: '2+', key: 'sqrt', expected: '2+sqrt(' },
    { expression: '2', key: '^', expected: '2^' },
    { expression: '50', key: '%', expected: '50%' },
    { expression: '8', key: '/', expected: '8/' },
    { expression: '8', key: '-', expected: '8-' },
    { expression: '8', key: '+', expected: '8+' },
    { expression: '(1', key: ')', expected: '(1)' },
  ] as const)('appends $key to "$expression"', ({ expression, key, expected }) => {
    expect(insertKey(expression, key)).toBe(expected)
  })

  it('offers every keypad token', () => {
    expect(KEYPAD_KEYS).toEqual([
      '0',
      '1',
      '2',
      '3',
      '4',
      '5',
      '6',
      '7',
      '8',
      '9',
      '.',
      '(',
      ')',
      '+',
      '-',
      '*',
      '/',
      '^',
      '%',
      'sqrt',
    ])
  })

  it('ignores an insert that would exceed the length limit (FR-10.4)', () => {
    const almostFull = '1'.repeat(MAX_EXPRESSION_LENGTH - 4)
    expect(insertKey(almostFull, 'sqrt')).toBe(almostFull)

    const oneShort = '1'.repeat(MAX_EXPRESSION_LENGTH - 1)
    const full = insertKey(oneShort, '1')
    expect(full).toHaveLength(MAX_EXPRESSION_LENGTH)
    expect(insertKey(full, '1')).toBe(full)
  })

  it('counts code points, not UTF-16 units', () => {
    const emoji = '😀'.repeat(MAX_EXPRESSION_LENGTH - 1)
    expect(insertKey(emoji, '1')).toBe(`${emoji}1`)
    expect(insertKey(`${emoji}1`, '1')).toBe(`${emoji}1`)
  })
})

describe('removeLastCharacter', () => {
  it.each([
    { expression: '12+3', expected: '12+' },
    { expression: 'sqrt(', expected: 'sqrt' },
    { expression: '1', expected: '' },
    { expression: '', expected: '' },
    { expression: '2😀', expected: '2' },
  ])('turns "$expression" into "$expected" (FR-12.1)', ({ expression, expected }) => {
    expect(removeLastCharacter(expression)).toBe(expected)
  })
})

describe('wrapResult', () => {
  it.each([
    { result: '5', expected: '5' },
    { result: '-5', expected: '(-5)' },
    { result: '0', expected: '0' },
    { result: '-0.125', expected: '(-0.125)' },
    { result: '1267650600228229401496703205376', expected: '1267650600228229401496703205376' },
  ])('re-enters $result as $expected (FR-11.8)', ({ result, expected }) => {
    expect(wrapResult(result)).toBe(expected)
  })
})

describe('historyLabel', () => {
  it('joins the expression and the raw result', () => {
    expect(historyLabel({ expression: '2+2', result: '4' })).toBe('2+2 = 4')
    expect(historyLabel({ expression: '2-7', result: '-5' })).toBe('2-7 = -5')
  })
})

describe('isEmptyError', () => {
  it('recognises only a 400 VALIDATION_FAILED whose first error is EMPTY', () => {
    expect(isEmptyError(emptyError)).toBe(true)
    expect(isEmptyError(validation('REQUIRED', 'expression is required'))).toBe(false)
    expect(isEmptyError(httpError(400, 'MALFORMED_REQUEST'))).toBe(false)
    expect(isEmptyError(httpError(422, 'EMPTY'))).toBe(false)
    expect(isEmptyError(httpError(400, 'VALIDATION_FAILED'))).toBe(false)
    expect(isEmptyError(networkError)).toBe(false)
  })
})

describe('messageFor', () => {
  it.each([
    { code: 'EMPTY', message: 'expression is empty' },
    { code: 'REQUIRED', message: 'expression is required' },
    { code: 'TOO_LONG', message: 'expression exceeds 1,024 characters' },
    { code: 'TOO_DEEP', message: 'expression is nested deeper than 32 levels' },
    { code: 'INVALID_CHARACTER', message: "invalid character '$' at character 2", position: 1 },
    { code: 'INVALID_NUMBER', message: 'invalid number at character 1', position: 0 },
    { code: 'UNEXPECTED_TOKEN', message: "unexpected ')' at character 4", position: 3 },
    { code: 'UNBALANCED_PARENTHESIS', message: "unbalanced ')' at character 4", position: 3 },
    { code: 'UNKNOWN_FUNCTION', message: "unknown function 'foo' at character 1", position: 0 },
  ])('shows the server message verbatim for $code (FR-13.2)', ({ code, message, position }) => {
    expect(messageFor(validation(code, message, position))).toBe(message)
  })

  it.each([
    { code: 'DIVISION_BY_ZERO', expected: 'Cannot divide by zero.' },
    {
      code: 'NEGATIVE_SQUARE_ROOT',
      expected: 'Cannot take the square root of a negative number.',
    },
    { code: 'INVALID_POWER', expected: 'Cannot raise a negative number to a fractional power.' },
    { code: 'EXPONENT_TOO_LARGE', expected: 'Exponent must be between -1000 and 1000.' },
    { code: 'RESULT_TOO_LARGE', expected: 'Result is too large to calculate.' },
  ])('words $code in plain language (FR-13.7)', ({ code, expected }) => {
    expect(messageFor(httpError(422, code))).toBe(expected)
  })

  it.each([
    { name: 'a network failure', error: networkError },
    { name: 'a timeout', error: new ApiError('timeout', 'slow') },
    { name: 'a 500', error: httpError(500, 'INTERNAL_ERROR') },
    { name: 'a 503 TIMEOUT', error: httpError(503, 'TIMEOUT') },
    { name: 'a 502 without a problem body', error: new ApiError('http', 'x', { status: 502 }) },
  ])('says the service is unavailable for $name (FR-13.1, FR-13.4)', ({ error }) => {
    expect(messageFor(error)).toBe(UNAVAILABLE)
  })

  it.each([
    { name: 'a 400 MALFORMED_REQUEST', error: httpError(400, 'MALFORMED_REQUEST') },
    { name: 'a 413 without a problem body', error: new ApiError('http', 'x', { status: 413 }) },
    { name: 'a 415', error: httpError(415, 'UNSUPPORTED_MEDIA_TYPE') },
    { name: 'a 404', error: httpError(404, 'NOT_FOUND') },
    { name: 'a 422 with an unknown code', error: httpError(422, 'NEW_CODE') },
    { name: 'a 400 VALIDATION_FAILED without errors', error: httpError(400, 'VALIDATION_FAILED') },
    {
      name: 'a 400 VALIDATION_FAILED with an empty errors list',
      error: httpError(400, 'VALIDATION_FAILED', { errors: [] }),
    },
    { name: 'an invalid response', error: new ApiError('invalid-response', 'x') },
    { name: 'an aborted request', error: new ApiError('aborted', 'x') },
  ])('falls back to the generic message for $name (FR-13.3)', ({ error }) => {
    expect(messageFor(error)).toBe(GENERIC)
  })
})

describe('calculatorReducer', () => {
  describe('editing', () => {
    it('starts a live evaluation when text is entered', () => {
      const state = calculatorReducer(initialState, { type: 'edited', text: '2+2' })
      expect(state).toMatchObject({ expression: '2+2', phase: 'live', result: null })
      expect(state.revision).toBe(initialState.revision + 1)
    })

    it('keeps the previous result and normalized expression while the next request is pending', () => {
      const shown = afterLive('2*(3+4', { expression: '2*(3+4)', result: '14' })
      const editing = calculatorReducer(shown, { type: 'edited', text: '2*(3+4)+' })
      expect(editing).toMatchObject({ result: '14', evaluatedAs: '2*(3+4)', phase: 'live' })
    })

    it('hides "Evaluated as" once the input matches the normalized expression', () => {
      const shown = afterLive('2*(3+4', { expression: '2*(3+4)', result: '14' })
      const editing = calculatorReducer(shown, { type: 'edited', text: '2*(3+4)' })
      expect(editing.evaluatedAs).toBeNull()
      expect(editing.result).toBe('14')
    })

    it.each(['', '   ', '\t'])('blanks everything but history for %j (FR-9.4)', (text) => {
      const failed = reduce([
        { type: 'edited', text: '1/0' },
        { type: 'commitRequested' },
        { type: 'commitFailed', error: divisionByZero },
      ])
      const state = calculatorReducer(
        { ...failed, history: [{ expression: '4', result: '4' }] },
        { type: 'edited', text },
      )
      expect(state).toMatchObject({
        expression: text,
        phase: 'idle',
        result: null,
        evaluatedAs: null,
        message: null,
        invalid: false,
        calculating: false,
        history: [{ expression: '4', result: '4' }],
      })
    })

    it('clears the alert and aria-invalid on any edit (FR-13.6)', () => {
      const failed = reduce([
        { type: 'edited', text: '1/0' },
        { type: 'commitRequested' },
        { type: 'commitFailed', error: divisionByZero },
      ])
      expect(failed).toMatchObject({ invalid: true, message: { kind: 'alert' } })

      const edited = calculatorReducer(failed, { type: 'edited', text: '1/02' })
      expect(edited).toMatchObject({ invalid: false, message: null, phase: 'live' })
    })

    it('keeps a status message until the next response replaces it', () => {
      const failed = reduce([
        { type: 'edited', text: '2+3)' },
        { type: 'requestStarted' },
        {
          type: 'liveFailed',
          error: validation('UNBALANCED_PARENTHESIS', "unbalanced ')' at character 4", 3),
        },
      ])
      const edited = calculatorReducer(failed, { type: 'edited', text: '2+3' })
      expect(edited.message).toEqual({ kind: 'status', text: "unbalanced ')' at character 4" })
    })

    it('resets "Calculating…" when the pending request is superseded', () => {
      const slow = reduce([
        { type: 'edited', text: '2+2' },
        { type: 'requestStarted' },
        { type: 'slow' },
      ])
      expect(slow.calculating).toBe(true)
      expect(calculatorReducer(slow, { type: 'edited', text: '2+2*' }).calculating).toBe(false)
    })

    it('aborts a commit and returns to the live path when edited during it (D-4)', () => {
      const committing = reduce([{ type: 'edited', text: '2+2' }, { type: 'commitRequested' }])
      const edited = calculatorReducer(committing, { type: 'edited', text: '2+2*' })
      expect(edited.phase).toBe('live')
      expect(
        calculatorReducer(edited, {
          type: 'commitSucceeded',
          response: { expression: '2+2', result: '4' },
        }),
      ).toBe(edited)
    })

    it('appends keypad tokens and ignores inserts beyond the limit', () => {
      const typed = reduce([
        { type: 'keyPressed', key: '7' },
        { type: 'keyPressed', key: '*' },
        { type: 'keyPressed', key: '(' },
        { type: 'keyPressed', key: '2' },
        { type: 'keyPressed', key: '+' },
        { type: 'keyPressed', key: '1' },
        { type: 'keyPressed', key: ')' },
      ])
      expect(typed).toMatchObject({ expression: '7*(2+1)', phase: 'live' })

      const full: CalculatorState = {
        ...initialState,
        expression: '1'.repeat(MAX_EXPRESSION_LENGTH),
      }
      expect(calculatorReducer(full, { type: 'keyPressed', key: 'sqrt' })).toBe(full)
    })

    it('removes the last character on backspace and ignores it when empty (FR-12.1)', () => {
      const typed = reduce([{ type: 'edited', text: '12+3' }, { type: 'backspace' }])
      expect(typed).toMatchObject({ expression: '12+', phase: 'live' })
      expect(reduce([{ type: 'edited', text: 'sqrt(' }, { type: 'backspace' }]).expression).toBe(
        'sqrt',
      )
      expect(reduce([{ type: 'edited', text: '1' }, { type: 'backspace' }])).toMatchObject({
        expression: '',
        phase: 'idle',
      })
      expect(calculatorReducer(initialState, { type: 'backspace' })).toBe(initialState)
    })
  })

  describe('live evaluation', () => {
    it('shows the result and clears any message on success', () => {
      const state = afterLive('2+2', { expression: '2+2', result: '4' })
      expect(state).toMatchObject({
        phase: 'idle',
        result: '4',
        evaluatedAs: null,
        message: null,
        calculating: false,
      })
    })

    it('reports the normalized expression only when it differs from the trimmed input (FR-9.7)', () => {
      expect(afterLive('2*(3+4', { expression: '2*(3+4)', result: '14' }).evaluatedAs).toBe(
        '2*(3+4)',
      )
      expect(afterLive('  2+2 ', { expression: '2+2', result: '4' }).evaluatedAs).toBeNull()
    })

    it('shows validation and arithmetic errors as status text and blanks the result', () => {
      const shown = afterLive('2+2', { expression: '2+2', result: '4' })
      const failed = reduce(
        [
          { type: 'edited', text: '2+2)' },
          { type: 'requestStarted' },
          {
            type: 'liveFailed',
            error: validation('UNBALANCED_PARENTHESIS', "unbalanced ')' at character 4", 3),
          },
        ],
        shown,
      )
      expect(failed).toMatchObject({
        phase: 'idle',
        result: null,
        evaluatedAs: null,
        invalid: false,
        calculating: false,
        message: { kind: 'status', text: "unbalanced ')' at character 4" },
      })
    })

    it('treats 400 EMPTY as a blank result with no message (FR-9.4)', () => {
      const shown = afterLive('2+2', { expression: '2+2', result: '4' })
      const failed = reduce(
        [
          { type: 'edited', text: '(' },
          { type: 'requestStarted' },
          { type: 'liveFailed', error: emptyError },
        ],
        shown,
      )
      expect(failed).toMatchObject({ phase: 'idle', result: null, message: null, invalid: false })
    })

    it('shows the network message as status text while typing, never as an alert (FR-13.1)', () => {
      const failed = reduce([
        { type: 'edited', text: '2+2' },
        { type: 'requestStarted' },
        { type: 'liveFailed', error: networkError },
      ])
      expect(failed.message).toEqual({ kind: 'status', text: UNAVAILABLE })
      expect(failed.invalid).toBe(false)
    })

    it('ignores live responses that arrive outside the live phase (FR-9.3)', () => {
      const committed = reduce([
        { type: 'edited', text: '2+2' },
        { type: 'commitRequested' },
        { type: 'commitSucceeded', response: { expression: '2+2', result: '4' } },
      ])
      expect(
        calculatorReducer(committed, {
          type: 'liveSucceeded',
          response: { expression: '2+2', result: '4' },
        }),
      ).toBe(committed)
      expect(calculatorReducer(committed, { type: 'liveFailed', error: networkError })).toBe(
        committed,
      )
      expect(calculatorReducer(initialState, { type: 'slow' })).toBe(initialState)
    })

    it('replaces the result and any status text with "Calculating…" after the slow timer (FR-9.6)', () => {
      const shown = afterLive('2+2', { expression: '2+2', result: '4' })
      const pending = reduce([{ type: 'edited', text: '2+2*3' }, { type: 'requestStarted' }], shown)
      expect(pending).toMatchObject({ result: '4', calculating: false })

      const slow = calculatorReducer(pending, { type: 'slow' })
      expect(slow).toMatchObject({
        result: null,
        evaluatedAs: null,
        message: null,
        calculating: true,
      })

      const done = calculatorReducer(slow, {
        type: 'liveSucceeded',
        response: { expression: '2+2*3', result: '8' },
      })
      expect(done).toMatchObject({ result: '8', calculating: false })
    })
  })

  describe('committing', () => {
    it('enters the committing phase and drops stale alerts', () => {
      const failed = reduce([
        { type: 'edited', text: '1/0' },
        { type: 'commitRequested' },
        { type: 'commitFailed', error: divisionByZero },
      ])
      const again = calculatorReducer(failed, { type: 'commitRequested' })
      expect(again).toMatchObject({
        phase: 'committing',
        message: null,
        invalid: false,
        calculating: false,
      })
    })

    it('keeps the previous result visible while the commit is pending', () => {
      const shown = afterLive('2+2', { expression: '2+2', result: '4' })
      expect(calculatorReducer(shown, { type: 'commitRequested' }).result).toBe('4')
    })

    it.each(['', '  '])('ignores a commit of %j (FR-11.5)', (text) => {
      const state = calculatorReducer(initialState, { type: 'edited', text })
      expect(calculatorReducer(state, { type: 'commitRequested' })).toBe(state)
    })

    it('ignores a second commit while one is in flight (FR-11.4)', () => {
      const committing = reduce([{ type: 'edited', text: '2+2' }, { type: 'commitRequested' }])
      expect(calculatorReducer(committing, { type: 'commitRequested' })).toBe(committing)
    })

    it('puts the result into the input, records history and goes quiet (FR-11.1, FR-11.3, FR-11.7)', () => {
      const state = reduce([
        { type: 'edited', text: '2*(3+4' },
        { type: 'requestStarted' },
        { type: 'liveSucceeded', response: { expression: '2*(3+4)', result: '14' } },
        { type: 'commitRequested' },
        { type: 'commitSucceeded', response: { expression: '2*(3+4)', result: '14' } },
      ])
      expect(state).toMatchObject({
        expression: '14',
        phase: 'committed',
        result: null,
        evaluatedAs: null,
        message: null,
        invalid: false,
        calculating: false,
        history: [{ expression: '2*(3+4)', result: '14' }],
      })
    })

    it('wraps negative results and stores the raw result in history (FR-11.8)', () => {
      const state = reduce([
        { type: 'edited', text: '2-7' },
        { type: 'commitRequested' },
        { type: 'commitSucceeded', response: { expression: '2-7', result: '-5' } },
      ])
      expect(state.expression).toBe('(-5)')
      expect(state.history).toEqual([{ expression: '2-7', result: '-5' }])

      const squared = reduce(
        [
          { type: 'edited', text: '(-5)^2' },
          { type: 'commitRequested' },
          { type: 'commitSucceeded', response: { expression: '(-5)^2', result: '25' } },
        ],
        state,
      )
      expect(squared.expression).toBe('25')
      expect(squared.history[0]).toEqual({ expression: '(-5)^2', result: '25' })
    })

    it('appends every commit, newest first, duplicates included (FR-11.9, FR-14.1)', () => {
      const commit = (text: string, result: string): CalculatorEvent[] => [
        { type: 'edited', text },
        { type: 'commitRequested' },
        { type: 'commitSucceeded', response: { expression: text, result } },
      ]
      const state = reduce([...commit('4', '4'), ...commit('2+2', '4'), ...commit('2+2', '4')])
      expect(state.history).toEqual([
        { expression: '2+2', result: '4' },
        { expression: '2+2', result: '4' },
        { expression: '4', result: '4' },
      ])
    })

    it('keeps only the newest 20 entries (FR-14.3)', () => {
      let state = initialState
      for (let i = 1; i <= MAX_HISTORY + 1; i += 1) {
        state = reduce(
          [
            { type: 'edited', text: `${i}` },
            { type: 'commitRequested' },
            { type: 'commitSucceeded', response: { expression: `${i}`, result: `${i}` } },
          ],
          state,
        )
      }
      expect(state.history).toHaveLength(MAX_HISTORY)
      expect(state.history[0]).toEqual({ expression: '21', result: '21' })
      expect(state.history[MAX_HISTORY - 1]).toEqual({ expression: '2', result: '2' })
    })

    it('keeps the input, blanks the result and raises an alert on an arithmetic error (FR-11.2, FR-13.5)', () => {
      const shown = afterLive('2+2', { expression: '2+2', result: '4' })
      const state = reduce(
        [
          { type: 'edited', text: '1/0' },
          { type: 'commitRequested' },
          { type: 'commitFailed', error: divisionByZero },
        ],
        shown,
      )
      expect(state).toMatchObject({
        expression: '1/0',
        phase: 'committed',
        result: null,
        evaluatedAs: null,
        invalid: true,
        calculating: false,
        message: { kind: 'alert', text: 'Cannot divide by zero.' },
        history: [],
      })
    })

    it('marks the input invalid for a committed validation error', () => {
      const state = reduce([
        { type: 'edited', text: '2+3)' },
        { type: 'commitRequested' },
        {
          type: 'commitFailed',
          error: validation('UNBALANCED_PARENTHESIS', "unbalanced ')' at character 4", 3),
        },
      ])
      expect(state).toMatchObject({
        invalid: true,
        message: { kind: 'alert', text: "unbalanced ')' at character 4" },
      })
    })

    it.each([
      { name: 'a network failure', error: networkError },
      { name: 'a timeout', error: new ApiError('timeout', 'slow') },
      { name: 'a 500', error: httpError(500, 'INTERNAL_ERROR') },
      { name: 'a 413', error: new ApiError('http', 'x', { status: 413 }) },
      { name: 'an invalid response', error: new ApiError('invalid-response', 'x') },
    ])('alerts without aria-invalid for $name (FR-13.5, D-3)', ({ error }) => {
      const state = reduce([
        { type: 'edited', text: '2+2' },
        { type: 'commitRequested' },
        { type: 'commitFailed', error },
      ])
      expect(state.invalid).toBe(false)
      expect(state.message).toEqual({ kind: 'alert', text: messageFor(error) })
      expect(state.expression).toBe('2+2')
    })

    it('treats a committed 400 EMPTY as a blank result with no alert or history (FR-11.6, A-8)', () => {
      const state = reduce([
        { type: 'edited', text: '(' },
        { type: 'commitRequested' },
        { type: 'commitFailed', error: emptyError },
      ])
      expect(state).toMatchObject({
        expression: '(',
        phase: 'committed',
        result: null,
        message: null,
        invalid: false,
        history: [],
      })
    })

    it('ignores commit responses outside the committing phase', () => {
      const live = calculatorReducer(initialState, { type: 'edited', text: '2+2' })
      expect(
        calculatorReducer(live, {
          type: 'commitSucceeded',
          response: { expression: '2+2', result: '4' },
        }),
      ).toBe(live)
      expect(calculatorReducer(live, { type: 'commitFailed', error: divisionByZero })).toBe(live)
    })

    it('shows "Calculating…" during a slow commit as well', () => {
      const shown = afterLive('2+2', { expression: '2+2', result: '4' })
      const slow = reduce(
        [{ type: 'commitRequested' }, { type: 'requestStarted' }, { type: 'slow' }],
        shown,
      )
      expect(slow).toMatchObject({ phase: 'committing', calculating: true, result: null })
    })
  })

  describe('clearing', () => {
    it('resets everything except history and aborts nothing on its own (FR-12.2)', () => {
      const busy = reduce([
        { type: 'edited', text: '2+2' },
        { type: 'commitRequested' },
        { type: 'commitSucceeded', response: { expression: '2+2', result: '4' } },
        { type: 'edited', text: '1/0' },
        { type: 'commitRequested' },
        { type: 'commitFailed', error: divisionByZero },
        { type: 'edited', text: '1/0+' },
        { type: 'requestStarted' },
        { type: 'slow' },
      ])
      const cleared = calculatorReducer(busy, { type: 'cleared' })
      expect(cleared).toEqual({
        ...initialState,
        revision: cleared.revision,
        history: [{ expression: '2+2', result: '4' }],
      })
      expect(cleared.revision).toBe(busy.revision + 1)
    })
  })

  describe('history', () => {
    const withHistory = reduce([
      { type: 'edited', text: '2+2' },
      { type: 'commitRequested' },
      { type: 'commitSucceeded', response: { expression: '2+2', result: '4' } },
      { type: 'edited', text: '3*3' },
      { type: 'commitRequested' },
      { type: 'commitSucceeded', response: { expression: '3*3', result: '9' } },
    ])

    it('loads the entry into the input and restarts the live path (FR-14.2)', () => {
      const state = calculatorReducer(withHistory, { type: 'historyActivated', index: 1 })
      expect(state).toMatchObject({ expression: '2+2', phase: 'live', result: null, message: null })
      expect(state.history).toBe(withHistory.history)
    })

    it('behaves like an edit even when the text is unchanged (A-9)', () => {
      const failed = reduce(
        [
          { type: 'edited', text: '3*3' },
          { type: 'commitRequested' },
          { type: 'commitFailed', error: httpError(500, 'INTERNAL_ERROR') },
        ],
        withHistory,
      )
      const state = calculatorReducer(failed, { type: 'historyActivated', index: 0 })
      expect(state).toMatchObject({
        expression: '3*3',
        phase: 'live',
        message: null,
        invalid: false,
      })
      expect(state.revision).toBe(failed.revision + 1)
    })

    it('ignores an index outside the list', () => {
      expect(calculatorReducer(withHistory, { type: 'historyActivated', index: 2 })).toBe(
        withHistory,
      )
      expect(calculatorReducer(withHistory, { type: 'historyActivated', index: -1 })).toBe(
        withHistory,
      )
    })
  })

  it('leaves the state untouched for a requestStarted outside a request phase', () => {
    expect(calculatorReducer(initialState, { type: 'requestStarted' })).toBe(initialState)
  })
})
