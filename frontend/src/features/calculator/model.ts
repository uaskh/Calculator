import type { ApiError } from '../../api/errors'
import { isArithmeticCode, type ArithmeticCode, type EvaluateResponse } from '../../api/evaluate'

/** Maximum expression length in Unicode code points, the backend's `TOO_LONG` limit. */
export const MAX_EXPRESSION_LENGTH = 1024
/** Committed calculations kept in the session history. */
export const MAX_HISTORY = 20

export const KEYPAD_KEYS = [
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
] as const
export type KeypadKey = (typeof KEYPAD_KEYS)[number]

/** What each keypad key appends; every key is literal except `sqrt`, which opens its call. */
const KEY_TOKENS: Partial<Record<KeypadKey, string>> = { sqrt: 'sqrt(' }

export interface HistoryEntry {
  /** The normalized expression the backend evaluated. */
  expression: string
  /** The raw result string, unwrapped. */
  result: string
}

export interface Message {
  /** `status`: polite text while typing. `alert`: assertive text after a failed commit. */
  kind: 'status' | 'alert'
  text: string
}

/**
 * `idle`: nothing pending. `live`: the input changed and a debounced preview is due or in
 * flight. `committing`: a commit request is in flight. `committed`: a commit has answered
 * and no live request is sent until the next edit.
 */
export type Phase = 'idle' | 'live' | 'committing' | 'committed'

export interface CalculatorState {
  expression: string
  phase: Phase
  /** The last live result, or null when blank. */
  result: string | null
  /** The normalized expression of the last 200, only when it differs from the trimmed input. */
  evaluatedAs: string | null
  message: Message | null
  /** True only after a commit answered 400 or 422 (never for EMPTY, network or other failures). */
  invalid: boolean
  /** True once a request has been pending for 300 ms without a response. */
  calculating: boolean
  /** Newest first, at most MAX_HISTORY entries. */
  history: HistoryEntry[]
  /** Counts edit-like events so that reloading identical text still restarts the preview. */
  revision: number
}

export type CalculatorEvent =
  | { type: 'edited'; text: string }
  | { type: 'keyPressed'; key: KeypadKey }
  | { type: 'backspace' }
  | { type: 'cleared' }
  | { type: 'commitRequested' }
  | { type: 'requestStarted' }
  | { type: 'slow' }
  | { type: 'liveSucceeded'; response: EvaluateResponse }
  | { type: 'liveFailed'; error: ApiError }
  | { type: 'commitSucceeded'; response: EvaluateResponse }
  | { type: 'commitFailed'; error: ApiError }
  | { type: 'historyActivated'; index: number }

export const initialState: CalculatorState = {
  expression: '',
  phase: 'idle',
  result: null,
  evaluatedAs: null,
  message: null,
  invalid: false,
  calculating: false,
  history: [],
  revision: 0,
}

const codePointLength = (text: string): number => [...text].length

/** Appends a keypad token; an insert that would exceed the length limit is ignored. */
export function insertKey(expression: string, key: KeypadKey): string {
  const next = expression + (KEY_TOKENS[key] ?? key)
  return codePointLength(next) > MAX_EXPRESSION_LENGTH ? expression : next
}

/** Removes the last code point (`sqrt(` → `sqrt`); an empty expression stays empty. */
export function removeLastCharacter(expression: string): string {
  return [...expression].slice(0, -1).join('')
}

/** A committed result re-enters the expression verbatim, wrapped when negative (`(-5)`). */
export function wrapResult(result: string): string {
  return result.startsWith('-') ? `(${result})` : result
}

export function historyLabel(entry: HistoryEntry): string {
  return `${entry.expression} = ${entry.result}`
}

const UNAVAILABLE_MESSAGE = 'The calculator service is unavailable. Try again.'
const GENERIC_MESSAGE = 'Something went wrong. Try again.'

const ARITHMETIC_MESSAGES: Record<ArithmeticCode, string> = {
  DIVISION_BY_ZERO: 'Cannot divide by zero.',
  NEGATIVE_SQUARE_ROOT: 'Cannot take the square root of a negative number.',
  INVALID_POWER: 'Cannot raise a negative number to a fractional power.',
  EXPONENT_TOO_LARGE: 'Exponent must be between -1000 and 1000.',
  RESULT_TOO_LARGE: 'Result is too large to calculate.',
}

function isValidationFailure(error: ApiError): boolean {
  return error.kind === 'http' && error.status === 400 && error.code === 'VALIDATION_FAILED'
}

/** A 400 VALIDATION_FAILED whose only error is EMPTY: shown as a blank result, never as text. */
export function isEmptyError(error: ApiError): boolean {
  return isValidationFailure(error) && error.problem?.errors?.[0]?.code === 'EMPTY'
}

/** User-facing copy for a failed evaluation (spec §7 "Wording"). */
export function messageFor(error: ApiError): string {
  switch (error.kind) {
    case 'network':
    case 'timeout':
      return UNAVAILABLE_MESSAGE
    case 'http':
      break
    default:
      return GENERIC_MESSAGE
  }
  const status = error.status ?? 0
  if (status >= 500) return UNAVAILABLE_MESSAGE
  if (isValidationFailure(error)) {
    const message = error.problem?.errors?.[0]?.message
    return message ?? GENERIC_MESSAGE
  }
  if (status === 422 && isArithmeticCode(error.code)) return ARITHMETIC_MESSAGES[error.code]
  return GENERIC_MESSAGE
}

/** Whether a failed commit marks the input invalid: only server-rejected input (400/422). */
function marksInvalid(error: ApiError): boolean {
  return error.kind === 'http' && (error.status === 400 || error.status === 422)
}

function edited(state: CalculatorState, text: string): CalculatorState {
  const blank = text.trim() === ''
  return {
    ...state,
    expression: text,
    phase: blank ? 'idle' : 'live',
    result: blank ? null : state.result,
    evaluatedAs: blank || state.evaluatedAs === text.trim() ? null : state.evaluatedAs,
    message: blank || state.message?.kind === 'alert' ? null : state.message,
    invalid: false,
    calculating: false,
    revision: state.revision + 1,
  }
}

function inRequestPhase(state: CalculatorState): boolean {
  return state.phase === 'live' || state.phase === 'committing'
}

export function calculatorReducer(state: CalculatorState, event: CalculatorEvent): CalculatorState {
  switch (event.type) {
    case 'edited':
      return edited(state, event.text)
    case 'keyPressed': {
      const next = insertKey(state.expression, event.key)
      return next === state.expression ? state : edited(state, next)
    }
    case 'backspace':
      return state.expression === '' ? state : edited(state, removeLastCharacter(state.expression))
    case 'cleared':
      return { ...initialState, history: state.history, revision: state.revision + 1 }
    case 'commitRequested':
      if (state.phase === 'committing' || state.expression.trim() === '') return state
      return { ...state, phase: 'committing', message: null, invalid: false, calculating: false }
    case 'requestStarted':
      // A fresh request restarts the 300 ms rule; edits already reset the flag, so this is a
      // safety net for a request that starts while a stale "Calculating…" is showing.
      return inRequestPhase(state) && state.calculating ? { ...state, calculating: false } : state
    case 'slow':
      if (!inRequestPhase(state)) return state
      return { ...state, result: null, evaluatedAs: null, message: null, calculating: true }
    case 'liveSucceeded': {
      if (state.phase !== 'live') return state
      const { expression, result } = event.response
      return {
        ...state,
        phase: 'idle',
        result,
        evaluatedAs: expression === state.expression.trim() ? null : expression,
        message: null,
        invalid: false,
        calculating: false,
      }
    }
    case 'liveFailed':
      if (state.phase !== 'live') return state
      return {
        ...state,
        phase: 'idle',
        result: null,
        evaluatedAs: null,
        message: isEmptyError(event.error)
          ? null
          : { kind: 'status', text: messageFor(event.error) },
        invalid: false,
        calculating: false,
      }
    case 'commitSucceeded': {
      if (state.phase !== 'committing') return state
      const { expression, result } = event.response
      return {
        ...state,
        expression: wrapResult(result),
        phase: 'committed',
        result: null,
        evaluatedAs: null,
        message: null,
        invalid: false,
        calculating: false,
        history: [{ expression, result }, ...state.history].slice(0, MAX_HISTORY),
      }
    }
    case 'commitFailed': {
      if (state.phase !== 'committing') return state
      const empty = isEmptyError(event.error)
      return {
        ...state,
        phase: 'committed',
        result: null,
        evaluatedAs: null,
        message: empty ? null : { kind: 'alert', text: messageFor(event.error) },
        invalid: !empty && marksInvalid(event.error),
        calculating: false,
      }
    }
    case 'historyActivated': {
      const entry = state.history[event.index]
      return entry === undefined ? state : edited(state, entry.expression)
    }
  }
}
