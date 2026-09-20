import type { MouseEvent } from 'react'
import styles from './Keypad.module.css'
import type { KeypadKey } from './model'

interface KeypadProps {
  onKey: (key: KeypadKey) => void
  onBackspace: () => void
  onClear: () => void
  /** Disables `=` while a commit is in flight. */
  committing: boolean
}

type KeyAction = { kind: 'key'; key: KeypadKey } | { kind: 'backspace' } | { kind: 'clear' }

/** Visual group of a key (`data-group`); Keypad.module.css colours each group differently. */
type KeyGroup = 'digit' | 'operator' | 'action' | 'equals'

interface KeyDefinition {
  /** Visible label; the accessible name when `name` is absent (digits). */
  label: string
  /** aria-label for keys whose glyph is not self-explanatory to assistive technology. */
  name?: string
  /** What the key does; a `submit` key submits the surrounding form instead. */
  action: KeyAction | 'submit'
  group: KeyGroup
}

const operator = (label: string, name: string, value: KeypadKey): KeyDefinition => ({
  label,
  name,
  action: { kind: 'key', key: value },
  group: 'operator',
})
const digit = (label: KeypadKey): KeyDefinition => ({
  label,
  action: { kind: 'key', key: label },
  group: 'digit',
})

/** Spec §7 order, four per row; `=` spans the last two columns. */
const KEYS: readonly KeyDefinition[] = [
  { label: 'C', name: 'clear', action: { kind: 'clear' }, group: 'action' },
  { label: '⌫', name: 'backspace', action: { kind: 'backspace' }, group: 'action' },
  operator('(', 'open parenthesis', '('),
  operator(')', 'close parenthesis', ')'),
  digit('7'),
  digit('8'),
  digit('9'),
  operator('÷', 'divide', '/'),
  digit('4'),
  digit('5'),
  digit('6'),
  operator('×', 'multiply', '*'),
  digit('1'),
  digit('2'),
  digit('3'),
  operator('−', 'subtract', '-'),
  digit('0'),
  { label: '.', name: 'decimal point', action: { kind: 'key', key: '.' }, group: 'digit' },
  operator('%', 'percent', '%'),
  operator('+', 'add', '+'),
  operator('sqrt', 'sqrt, square root', 'sqrt'),
  operator('^', 'power', '^'),
  { label: '=', name: 'equals', action: 'submit', group: 'equals' },
]

/** Keeps focus where it is (the expression input) when a key is pressed with a pointer. */
function keepFocus(event: MouseEvent<HTMLButtonElement>) {
  event.preventDefault()
}

export function Keypad({ onKey, onBackspace, onClear, committing }: KeypadProps) {
  function activate(action: KeyAction) {
    switch (action.kind) {
      case 'key':
        onKey(action.key)
        break
      case 'backspace':
        onBackspace()
        break
      case 'clear':
        onClear()
        break
    }
  }

  return (
    <div className={styles.keypad}>
      {KEYS.map(({ label, name, action, group }) =>
        action === 'submit' ? (
          <button
            key={label}
            type="submit"
            className={`${styles.key} ${styles.equals}`}
            data-group={group}
            aria-label={name}
            disabled={committing}
            onMouseDown={keepFocus}
          >
            {label}
          </button>
        ) : (
          <button
            key={label}
            type="button"
            className={styles.key}
            data-group={group}
            aria-label={name}
            onMouseDown={keepFocus}
            onClick={() => {
              activate(action)
            }}
          >
            {label}
          </button>
        ),
      )}
    </div>
  )
}
