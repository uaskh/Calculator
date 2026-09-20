import { useEffect, useId, useRef, useState, type FormEvent, type KeyboardEvent } from 'react'
import { flushSync } from 'react-dom'
import styles from './Calculator.module.css'
import { History } from './History'
import { Keypad } from './Keypad'
import { MAX_EXPRESSION_LENGTH } from './model'
import { useCalculator } from './useCalculator'
import { useTypeAnywhere } from './useTypeAnywhere'

/** Touch devices get no soft keyboard: the keypad is the input method (spec §7, D25). */
function hasCoarsePointer(): boolean {
  return typeof window.matchMedia === 'function' && window.matchMedia('(pointer: coarse)').matches
}

export function Calculator() {
  const { state, edit, pressKey, typeCharacter, backspace, clear, commit, activateHistory } =
    useCalculator()
  const inputRef = useRef<HTMLInputElement>(null)
  const inputId = useId()
  const alertId = useId()
  const [coarsePointer] = useState(hasCoarsePointer)

  const committing = state.phase === 'committing'
  const alert = state.message?.kind === 'alert' ? state.message.text : null
  const status = state.message?.kind === 'status' ? state.message.text : ''
  const resultText = state.calculating ? 'Calculating…' : (state.result ?? '')

  function focusInputAtEnd() {
    const input = inputRef.current
    if (!input) return
    input.focus()
    const end = input.value.length
    input.setSelectionRange(end, end)
  }

  /** Applies a programmatic change and then puts the caret at the end of the new value. */
  function applyAndFocus(change: () => void) {
    flushSync(change)
    focusInputAtEnd()
  }

  function commitAndFocus() {
    commit()
    inputRef.current?.focus()
  }

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    commitAndFocus()
  }

  // The input is focused on load so typing works without a click (decision 38). An effect
  // rather than the `autoFocus` attribute: jsx-a11y forbids the attribute (no-autofocus).
  useEffect(() => {
    inputRef.current?.focus()
  }, [])

  useTypeAnywhere({
    onCharacter: (character) => {
      applyAndFocus(() => {
        typeCharacter(character)
      })
    },
    onBackspace: () => {
      applyAndFocus(backspace)
    },
    onClear: () => {
      applyAndFocus(clear)
    },
    onCommit: commitAndFocus,
  })

  function handleKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key !== 'Escape') return
    event.preventDefault()
    applyAndFocus(clear)
  }

  return (
    <div className={styles.calculator}>
      <form className={styles.form} noValidate onSubmit={handleSubmit}>
        <div className={styles.display}>
          <label className={styles.label} htmlFor={inputId}>
            Expression
          </label>
          <input
            ref={inputRef}
            id={inputId}
            className={styles.input}
            type="text"
            name="expression"
            value={state.expression}
            maxLength={MAX_EXPRESSION_LENGTH}
            autoComplete="off"
            spellCheck={false}
            inputMode={coarsePointer ? 'none' : 'text'}
            aria-invalid={state.invalid || undefined}
            aria-describedby={state.invalid ? alertId : undefined}
            onChange={(event) => {
              edit(event.target.value)
            }}
            onKeyDown={handleKeyDown}
          />
          <output
            className={styles.output}
            htmlFor={inputId}
            aria-label="Result"
            aria-live="polite"
            // Focusable so keyboard users can scroll a long result (axe: scrollable-region-focusable).
            // eslint-disable-next-line jsx-a11y/no-noninteractive-tabindex -- scrollable region
            tabIndex={0}
          >
            {resultText}
          </output>
          {state.evaluatedAs !== null && (
            <p className={styles.evaluatedAs}>Evaluated as {state.evaluatedAs}</p>
          )}
          <p className={styles.status} role="status">
            {status}
          </p>
          {alert !== null && (
            <p id={alertId} className={styles.alert} role="alert">
              {alert}
            </p>
          )}
        </div>
        <div className={styles.keypadColumn}>
          <Keypad
            committing={committing}
            onKey={(key) => {
              applyAndFocus(() => {
                pressKey(key)
              })
            }}
            onBackspace={() => {
              applyAndFocus(backspace)
            }}
            onClear={() => {
              applyAndFocus(clear)
            }}
          />
        </div>
      </form>
      {/* Reserves the history column on desktop even while History renders nothing. */}
      <div className={styles.historyColumn}>
        <History
          entries={state.history}
          onActivate={(index) => {
            applyAndFocus(() => {
              activateHistory(index)
            })
          }}
        />
      </div>
    </div>
  )
}
