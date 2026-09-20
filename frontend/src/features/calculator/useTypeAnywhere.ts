import { useEffect, useEffectEvent } from 'react'

/** What the calculator does with a key pressed outside its input (spec §7, decision 38). */
export type TypeAnywhereAction =
  | { kind: 'character'; character: string }
  | { kind: 'backspace' }
  | { kind: 'clear' }
  | { kind: 'commit' }

export interface TypeAnywhereHandlers {
  onCharacter: (character: string) => void
  onBackspace: () => void
  onClear: () => void
  onCommit: () => void
}

type RoutableKeyEvent = Pick<
  KeyboardEvent,
  'key' | 'ctrlKey' | 'metaKey' | 'altKey' | 'isComposing' | 'target'
>

const EDITABLE_TAGS = new Set(['INPUT', 'TEXTAREA', 'SELECT'])

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
 * Enter does only from the page background, because Enter and Space on a button must keep
 * their native activation. Modifier combinations and composition keys are never taken.
 */
export function routeKey(event: RoutableKeyEvent): TypeAnywhereAction | null {
  if (event.ctrlKey || event.metaKey || event.altKey || event.isComposing) return null
  const { target } = event
  if (!(target instanceof HTMLElement) || isEditable(target)) return null
  const onButton = target instanceof HTMLButtonElement
  switch (event.key) {
    case 'Backspace':
      return { kind: 'backspace' }
    case 'Escape':
      return { kind: 'clear' }
    case 'Enter':
      return onButton ? null : { kind: 'commit' }
    case ' ':
      return onButton ? null : { kind: 'character', character: ' ' }
    default:
      return event.key.length === 1 ? { kind: 'character', character: event.key } : null
  }
}

/**
 * Lets the user type into the calculator from anywhere on the page: one document-level
 * keydown listener for the component's lifetime routes keys through `routeKey` and
 * prevents the default of every key it takes.
 */
export function useTypeAnywhere(handlers: TypeAnywhereHandlers): void {
  const handleKeyDown = useEffectEvent((event: KeyboardEvent) => {
    const action = routeKey(event)
    if (action === null) return
    event.preventDefault()
    switch (action.kind) {
      case 'character':
        handlers.onCharacter(action.character)
        break
      case 'backspace':
        handlers.onBackspace()
        break
      case 'clear':
        handlers.onClear()
        break
      case 'commit':
        handlers.onCommit()
        break
    }
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
