import { renderHook } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { routeKey, useTypeAnywhere, type TypeAnywhereHandlers } from './useTypeAnywhere'

type KeyInit = Pick<KeyboardEventInit, 'key' | 'ctrlKey' | 'metaKey' | 'altKey' | 'isComposing'>

const ALL_HANDLERS = ['onCharacter', 'onBackspace', 'onClear', 'onCommit', 'onFocus'] as const

/** Dispatches a cancelable keydown on `target` and reports whether it was prevented. */
function press(target: EventTarget, init: KeyInit): boolean {
  const event = new KeyboardEvent('keydown', { bubbles: true, cancelable: true, ...init })
  target.dispatchEvent(event)
  return event.defaultPrevented
}

function mount(tag: string, attributes: Record<string, string> = {}): HTMLElement {
  const element = document.createElement(tag)
  for (const [name, value] of Object.entries(attributes)) element.setAttribute(name, value)
  document.body.append(element)
  return element
}

function handlers(): TypeAnywhereHandlers {
  return {
    onCharacter: vi.fn<(character: string) => void>(),
    onBackspace: vi.fn<() => void>(),
    onClear: vi.fn<() => void>(),
    onCommit: vi.fn<() => void>(),
    onFocus: vi.fn<() => void>(),
  }
}

function expectNoHandlerCalled(h: TypeAnywhereHandlers) {
  for (const name of ALL_HANDLERS) expect(h[name], name).not.toHaveBeenCalled()
}

describe('routeKey (decision 38)', () => {
  afterEach(() => {
    document.body.innerHTML = ''
  })

  it.each([
    { key: '7', expected: { kind: 'character', character: '7' } },
    { key: '+', expected: { kind: 'character', character: '+' } },
    { key: ' ', expected: { kind: 'character', character: ' ' } },
    { key: 'Backspace', expected: { kind: 'backspace' } },
    { key: 'Escape', expected: { kind: 'clear' } },
    { key: 'Enter', expected: { kind: 'commit' } },
    { key: 'Tab', expected: null },
    { key: 'Shift', expected: null },
    { key: 'ArrowLeft', expected: null },
    { key: 'Dead', expected: { kind: 'focus' } },
    { key: 'Process', expected: null },
  ])('routes $key from the page background to $expected', ({ key, expected }) => {
    const event = new KeyboardEvent('keydown', { key })
    document.body.dispatchEvent(event)

    expect(routeKey(event)).toEqual(expected)
  })

  it.each([
    { key: 'Enter', expected: null },
    { key: ' ', expected: null },
    { key: '7', expected: { kind: 'character', character: '7' } },
    { key: 'Backspace', expected: { kind: 'backspace' } },
    { key: 'Escape', expected: { kind: 'clear' } },
    { key: 'Tab', expected: null },
  ])('routes $key from a button to $expected', ({ key, expected }) => {
    const button = mount('button')
    const event = new KeyboardEvent('keydown', { key, bubbles: true })
    button.dispatchEvent(event)

    expect(routeKey(event)).toEqual(expected)
  })

  it.each([
    { name: 'a link', element: () => mount('a', { href: '#top' }) },
    { name: 'a summary', element: () => mount('summary') },
    { name: 'an ARIA button', element: () => mount('span', { role: 'button', tabindex: '0' }) },
    { name: 'an ARIA link', element: () => mount('span', { role: 'link', tabindex: '0' }) },
    { name: 'a menu item', element: () => mount('li', { role: 'menuitem', tabindex: '0' }) },
    {
      name: 'a child of a link',
      element: () => {
        const child = document.createElement('span')
        mount('a', { href: '#top' }).append(child)
        return child
      },
    },
  ])('leaves Enter and Space to $name but still routes characters', ({ element }) => {
    const target = element()
    for (const key of ['Enter', ' ']) {
      const event = new KeyboardEvent('keydown', { key, bubbles: true })
      target.dispatchEvent(event)
      expect(routeKey(event), key).toBeNull()
    }
    const seven = new KeyboardEvent('keydown', { key: '7', bubbles: true })
    target.dispatchEvent(seven)
    expect(routeKey(seven)).toEqual({ kind: 'character', character: '7' })
  })

  it('leaves a link without href to the page background', () => {
    const target = mount('a')
    const event = new KeyboardEvent('keydown', { key: 'Enter', bubbles: true })
    target.dispatchEvent(event)

    expect(routeKey(event)).toEqual({ kind: 'commit' })
  })

  it.each(['7', 'Backspace', 'Escape', 'Enter', ' ', 'Dead'])(
    'never takes %j once another listener has prevented it',
    (key) => {
      const event = new KeyboardEvent('keydown', { key, cancelable: true })
      event.preventDefault()
      document.body.dispatchEvent(event)

      expect(routeKey(event)).toBeNull()
    },
  )

  it('leaves a dead key in the expression input to the browser', () => {
    const input = mount('input', { type: 'text' })
    const event = new KeyboardEvent('keydown', { key: 'Dead', bubbles: true })
    input.dispatchEvent(event)

    expect(routeKey(event)).toBeNull()
  })

  it.each([
    { name: 'a text input', element: () => mount('input', { type: 'text' }) },
    { name: 'a textarea', element: () => mount('textarea') },
    { name: 'a select', element: () => mount('select') },
    { name: 'a contenteditable element', element: () => mount('div', { contenteditable: 'true' }) },
    {
      name: 'a child of a contenteditable element',
      element: () => {
        const child = document.createElement('span')
        mount('div', { contenteditable: '' }).append(child)
        return child
      },
    },
  ])('leaves every key to $name', ({ element }) => {
    const target = element()
    for (const key of ['7', 'Backspace', 'Escape', 'Enter', ' ']) {
      const event = new KeyboardEvent('keydown', { key, bubbles: true })
      target.dispatchEvent(event)
      expect(routeKey(event)).toBeNull()
    }
  })

  it('treats contenteditable="false" as a plain element', () => {
    const target = mount('div', { contenteditable: 'false' })
    const event = new KeyboardEvent('keydown', { key: '7', bubbles: true })
    target.dispatchEvent(event)

    expect(routeKey(event)).toEqual({ kind: 'character', character: '7' })
  })

  it.each([
    { name: 'Ctrl', init: { ctrlKey: true } },
    { name: 'Meta', init: { metaKey: true } },
    { name: 'Alt', init: { altKey: true } },
  ])('never intercepts $name combinations', ({ init }) => {
    for (const key of ['7', 'Backspace', 'Escape', 'Enter', 'r', 'c']) {
      const event = new KeyboardEvent('keydown', { key, ...init })
      document.body.dispatchEvent(event)
      expect(routeKey(event)).toBeNull()
    }
  })

  it('lets Shift through so shifted characters still type', () => {
    const event = new KeyboardEvent('keydown', { key: '(', shiftKey: true })
    document.body.dispatchEvent(event)

    expect(routeKey(event)).toEqual({ kind: 'character', character: '(' })
  })

  it('ignores keys that are part of an IME composition', () => {
    const event = new KeyboardEvent('keydown', { key: 'a', isComposing: true })
    document.body.dispatchEvent(event)

    expect(routeKey(event)).toBeNull()
  })

  it('ignores an event without an element target', () => {
    expect(routeKey(new KeyboardEvent('keydown', { key: '7' }))).toBeNull()
  })
})

describe('useTypeAnywhere (decision 38)', () => {
  afterEach(() => {
    document.body.innerHTML = ''
  })

  it('routes background keys to the handlers and prevents their default', () => {
    const h = handlers()
    renderHook(() => {
      useTypeAnywhere(h)
    })

    expect(press(document.body, { key: '7' })).toBe(true)
    expect(press(document.body, { key: 'Backspace' })).toBe(true)
    expect(press(document.body, { key: 'Escape' })).toBe(true)
    expect(press(document.body, { key: 'Enter' })).toBe(true)

    expect(h.onCharacter).toHaveBeenCalledExactlyOnceWith('7')
    expect(h.onBackspace).toHaveBeenCalledOnce()
    expect(h.onClear).toHaveBeenCalledOnce()
    expect(h.onCommit).toHaveBeenCalledOnce()
    expect(h.onFocus).not.toHaveBeenCalled()
  })

  it('focuses the input for a dead key on the background without preventing it', () => {
    const h = handlers()
    renderHook(() => {
      useTypeAnywhere(h)
    })

    expect(press(document.body, { key: 'Dead' })).toBe(false)

    expect(h.onFocus).toHaveBeenCalledOnce()
    expect(h.onCharacter).not.toHaveBeenCalled()
  })

  it('ignores a dead key inside the input', () => {
    const h = handlers()
    const input = mount('input', { type: 'text' })
    renderHook(() => {
      useTypeAnywhere(h)
    })

    expect(press(input, { key: 'Dead' })).toBe(false)
    expectNoHandlerCalled(h)
  })

  it('ignores a key another listener prevented first', () => {
    const h = handlers()
    renderHook(() => {
      useTypeAnywhere(h)
    })
    const first = (event: KeyboardEvent) => {
      event.preventDefault()
    }
    document.body.addEventListener('keydown', first)

    press(document.body, { key: '7' })

    document.body.removeEventListener('keydown', first)
    expectNoHandlerCalled(h)
  })

  it('leaves Enter and Space on a button, Tab and modifier combinations untouched', () => {
    const h = handlers()
    const button = mount('button')
    renderHook(() => {
      useTypeAnywhere(h)
    })

    expect(press(button, { key: 'Enter' })).toBe(false)
    expect(press(button, { key: ' ' })).toBe(false)
    expect(press(document.body, { key: 'Tab' })).toBe(false)
    expect(press(document.body, { key: 'r', metaKey: true })).toBe(false)
    expect(press(document.body, { key: 'c', ctrlKey: true })).toBe(false)

    expectNoHandlerCalled(h)
  })

  it('leaves Enter and Space on a link untouched', () => {
    const h = handlers()
    const link = mount('a', { href: '#top' })
    renderHook(() => {
      useTypeAnywhere(h)
    })

    expect(press(link, { key: 'Enter' })).toBe(false)
    expect(press(link, { key: ' ' })).toBe(false)

    expectNoHandlerCalled(h)
  })

  it('leaves keys in the expression input alone', () => {
    const h = handlers()
    const input = mount('input', { type: 'text' })
    renderHook(() => {
      useTypeAnywhere(h)
    })

    expect(press(input, { key: '7' })).toBe(false)
    expect(press(input, { key: 'Enter' })).toBe(false)
    expect(press(input, { key: 'Escape' })).toBe(false)
    expect(h.onCharacter).not.toHaveBeenCalled()
    expect(h.onCommit).not.toHaveBeenCalled()
    expect(h.onClear).not.toHaveBeenCalled()
  })

  it('uses the latest handlers without re-registering', () => {
    const first = handlers()
    const second = handlers()
    const addListener = vi.spyOn(document, 'addEventListener')
    const { rerender } = renderHook(
      ({ current }) => {
        useTypeAnywhere(current)
      },
      { initialProps: { current: first } },
    )
    rerender({ current: second })

    press(document.body, { key: '7' })

    expect(first.onCharacter).not.toHaveBeenCalled()
    expect(second.onCharacter).toHaveBeenCalledExactlyOnceWith('7')
    expect(addListener.mock.calls.filter(([type]) => type === 'keydown')).toHaveLength(1)
  })

  it('stops listening on unmount', () => {
    const h = handlers()
    const { unmount } = renderHook(() => {
      useTypeAnywhere(h)
    })
    unmount()

    expect(press(document.body, { key: '7' })).toBe(false)
    expect(h.onCharacter).not.toHaveBeenCalled()
  })
})
