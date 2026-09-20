import { useEffect, useEffectEvent } from 'react'

/** Payload of each action, keyed by kind. `routeKey` and the dispatch table share it. */
interface ActionPayloads {
  character: { character: string }
  backspace: Record<never, never>
  clear: Record<never, never>
  commit: Record<never, never>
  /** Moves focus into the input so the browser can finish a dead-key sequence there. */
  focus: Record<never, never>
}

type ActionKind = keyof ActionPayloads

type ActionOf<K extends ActionKind> = { [P in K]: { kind: P } & ActionPayloads[P] }[K]

/** What the calculator does with a key pressed outside its input (spec §7, decision 38). */
export type TypeAnywhereAction = ActionOf<ActionKind>

export interface TypeAnywhereHandlers {
  onCharacter: (character: string) => void
  onBackspace: () => void
  onClear: () => void
  onCommit: () => void
  /** Focuses the input with the caret at the end without changing the expression. */
  onFocus: () => void
}

type RoutableKeyEvent = Pick<
  KeyboardEvent,
  'key' | 'ctrlKey' | 'metaKey' | 'altKey' | 'isComposing' | 'defaultPrevented' | 'target'
>

const EDITABLE_TAGS = new Set(['INPUT', 'TEXTAREA', 'SELECT'])

/** Elements whose Enter and Space activate them natively, so neither key is ever taken. */
const ACTIVATABLE_SELECTOR =
  'button, a[href], summary, [role="button"], [role="link"], [role="menuitem"]'

/** Elements that own their own key handling: form fields and editable regions. */
function isEditable(element: HTMLElement): boolean {
  if (EDITABLE_TAGS.has(element.tagName)) return true
  if (element.isContentEditable) return true
  const region = element.closest('[contenteditable]')
  return region !== null && region.getAttribute('contenteditable') !== 'false'
}

/**
 * Decides whether a keydown that did not happen in an editable element belongs to the
 * calculator: printable characters, Backspace and Escape always do (also from a button);
 * Enter and Space do only from the page background, because on a button, link or other
 * activatable element they must keep their native activation. A dead key (`Dead`) only
 * moves focus into the input, where the browser completes the sequence. Modifier
 * combinations, composition keys and events another listener already prevented are never
 * taken.
 */
export function routeKey(event: RoutableKeyEvent): TypeAnywhereAction | null {
  if (event.defaultPrevented) return null
  if (event.ctrlKey || event.metaKey || event.altKey || event.isComposing) return null
  const { target } = event
  if (!(target instanceof HTMLElement) || isEditable(target)) return null
  const activatable = target.closest(ACTIVATABLE_SELECTOR) !== null
  switch (event.key) {
    case 'Backspace':
      return { kind: 'backspace' }
    case 'Escape':
      return { kind: 'clear' }
    case 'Dead':
      return { kind: 'focus' }
    case 'Enter':
      return activatable ? null : { kind: 'commit' }
    case ' ':
      return activatable ? null : { kind: 'character', character: ' ' }
    default:
      return event.key.length === 1 ? { kind: 'character', character: event.key } : null
  }
}

interface ActionDispatch<K extends ActionKind> {
  /** Whether the key's default must be suppressed once the calculator has taken it. */
  preventsDefault: boolean
  run: (action: ActionOf<K>) => void
}

type DispatchTable = { [K in ActionKind]: ActionDispatch<K> }

/** One entry per action kind; the mapped type keeps the table exhaustive at compile time. */
function dispatchTable(handlers: TypeAnywhereHandlers): DispatchTable {
  return {
    character: {
      preventsDefault: true,
      run: ({ character }) => {
        handlers.onCharacter(character)
      },
    },
    backspace: { preventsDefault: true, run: handlers.onBackspace },
    clear: { preventsDefault: true, run: handlers.onClear },
    commit: { preventsDefault: true, run: handlers.onCommit },
    // Not prevented: the browser must see the dead key to compose the next character.
    focus: { preventsDefault: false, run: handlers.onFocus },
  }
}

function dispatch<K extends ActionKind>(
  table: DispatchTable,
  event: KeyboardEvent,
  action: ActionOf<K>,
): void {
  const entry: ActionDispatch<K> = table[action.kind]
  if (entry.preventsDefault) event.preventDefault()
  entry.run(action)
}

/**
 * Lets the user type into the calculator from anywhere on the page: one document-level
 * keydown listener for the component's lifetime routes keys through `routeKey` and
 * prevents the default of every key it takes (except the dead-key focus hand-off).
 */
export function useTypeAnywhere(handlers: TypeAnywhereHandlers): void {
  const handleKeyDown = useEffectEvent((event: KeyboardEvent) => {
    const action = routeKey(event)
    if (action === null) return
    dispatch(dispatchTable(handlers), event, action)
  })

  useEffect(() => {
    const listener = (event: KeyboardEvent) => {
      handleKeyDown(event)
    }
    document.addEventListener('keydown', listener)
    return () => {
      document.removeEventListener('keydown', listener)
    }
  }, [])
}
