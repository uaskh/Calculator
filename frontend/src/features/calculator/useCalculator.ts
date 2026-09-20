import { useCallback, useEffect, useReducer } from 'react'
import { toApiError } from '../../api/errors'
import { evaluate } from '../../api/evaluate'
import { useApi } from '../../api/useApi'
import { calculatorReducer, initialState, type CalculatorState, type KeypadKey } from './model'

/** Quiet time after the last edit before the live request is sent. */
export const DEBOUNCE_MS = 150
/** Time a request may stay unanswered before "Calculating…" replaces the result. */
export const SLOW_MS = 300

export interface Calculator {
  state: CalculatorState
  edit: (text: string) => void
  pressKey: (key: KeypadKey) => void
  backspace: () => void
  clear: () => void
  commit: () => void
  activateHistory: (index: number) => void
}

/**
 * Orchestrates the calculator: the reducer decides, this hook schedules. Whenever the
 * phase, expression or revision changes, the previous request (or pending debounce) is
 * cancelled; a `live` phase schedules one request after the debounce, a `committing` phase
 * sends one immediately. Responses of cancelled requests never reach the reducer.
 */
export function useCalculator(): Calculator {
  const api = useApi()
  const [state, dispatch] = useReducer(calculatorReducer, initialState)
  const { phase, expression, revision } = state

  useEffect(() => {
    if ((phase !== 'live' && phase !== 'committing') || expression.trim() === '') return
    const live = phase === 'live'
    const controller = new AbortController()
    let slowTimer: ReturnType<typeof setTimeout> | undefined

    const run = async () => {
      dispatch({ type: 'requestStarted' })
      slowTimer = setTimeout(() => {
        dispatch({ type: 'slow' })
      }, SLOW_MS)
      try {
        const response = await evaluate(api, expression, { signal: controller.signal })
        if (controller.signal.aborted) return
        dispatch({ type: live ? 'liveSucceeded' : 'commitSucceeded', response })
      } catch (error) {
        if (controller.signal.aborted) return
        const apiError = toApiError(error)
        if (apiError.kind === 'aborted') return
        dispatch({ type: live ? 'liveFailed' : 'commitFailed', error: apiError })
      } finally {
        clearTimeout(slowTimer)
      }
    }

    // A commit starts on the next tick so that a cleanup (StrictMode, a synchronous edit)
    // can cancel it before anything is sent.
    const startTimer = setTimeout(() => void run(), live ? DEBOUNCE_MS : 0)
    return () => {
      clearTimeout(startTimer)
      clearTimeout(slowTimer)
      controller.abort()
    }
  }, [api, phase, expression, revision])

  const edit = useCallback((text: string) => {
    dispatch({ type: 'edited', text })
  }, [])
  const pressKey = useCallback((key: KeypadKey) => {
    dispatch({ type: 'keyPressed', key })
  }, [])
  const backspace = useCallback(() => {
    dispatch({ type: 'backspace' })
  }, [])
  const clear = useCallback(() => {
    dispatch({ type: 'cleared' })
  }, [])
  const commit = useCallback(() => {
    dispatch({ type: 'commitRequested' })
  }, [])
  const activateHistory = useCallback((index: number) => {
    dispatch({ type: 'historyActivated', index })
  }, [])

  return { state, edit, pressKey, backspace, clear, commit, activateHistory }
}
