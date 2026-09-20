import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { Keypad } from './Keypad'
import type { KeypadKey } from './model'

function renderKeypad(committing = false) {
  const onKey = vi.fn<(key: KeypadKey) => void>()
  const onBackspace = vi.fn<() => void>()
  const onClear = vi.fn<() => void>()
  const onSubmit = vi.fn((event: React.FormEvent) => {
    event.preventDefault()
  })
  render(
    <form onSubmit={onSubmit}>
      <Keypad onKey={onKey} onBackspace={onBackspace} onClear={onClear} committing={committing} />
    </form>,
  )
  return { onKey, onBackspace, onClear, onSubmit }
}

describe('<Keypad />', () => {
  it('renders the 23 keys in spec order with their accessible names (FR-10.1)', () => {
    renderKeypad()

    const names = screen
      .getAllByRole('button')
      .map((button) => button.getAttribute('aria-label') ?? button.textContent)
    expect(names).toEqual([
      'clear',
      'backspace',
      'open parenthesis',
      'close parenthesis',
      '7',
      '8',
      '9',
      'divide',
      '4',
      '5',
      '6',
      'multiply',
      '1',
      '2',
      '3',
      'subtract',
      '0',
      'decimal point',
      'percent',
      'add',
      'sqrt, square root',
      'power',
      'equals',
    ])
  })

  it('shows the visible glyphs the spec asks for', () => {
    renderKeypad()

    expect(screen.getByRole('button', { name: 'divide' })).toHaveTextContent('÷')
    expect(screen.getByRole('button', { name: 'multiply' })).toHaveTextContent('×')
    expect(screen.getByRole('button', { name: 'subtract' })).toHaveTextContent('−')
    expect(screen.getByRole('button', { name: 'backspace' })).toHaveTextContent('⌫')
    expect(screen.getByRole('button', { name: 'sqrt, square root' })).toHaveTextContent('sqrt')
    expect(screen.getByRole('button', { name: 'equals' })).toHaveTextContent('=')
  })

  it('makes only = a submit button', () => {
    renderKeypad()

    const buttons = screen.getAllByRole('button')
    const submits = buttons.filter((button) => button.getAttribute('type') === 'submit')
    expect(submits).toEqual([screen.getByRole('button', { name: 'equals' })])
    expect(buttons.filter((button) => button.getAttribute('type') === 'button')).toHaveLength(22)
  })

  it('reports ASCII tokens for the operator glyphs and sqrt for the square root (FR-10.2)', async () => {
    const user = userEvent.setup()
    const { onKey } = renderKeypad()

    for (const name of [
      '7',
      'multiply',
      'open parenthesis',
      '2',
      'add',
      '1',
      'close parenthesis',
    ]) {
      await user.click(screen.getByRole('button', { name }))
    }
    await user.click(screen.getByRole('button', { name: 'divide' }))
    await user.click(screen.getByRole('button', { name: 'subtract' }))
    await user.click(screen.getByRole('button', { name: 'sqrt, square root' }))
    await user.click(screen.getByRole('button', { name: 'power' }))
    await user.click(screen.getByRole('button', { name: 'percent' }))
    await user.click(screen.getByRole('button', { name: 'decimal point' }))

    expect(onKey.mock.calls.map(([key]) => key)).toEqual([
      '7',
      '*',
      '(',
      '2',
      '+',
      '1',
      ')',
      '/',
      '-',
      'sqrt',
      '^',
      '%',
      '.',
    ])
  })

  it('reports clear and backspace, and submits the form for =', async () => {
    const user = userEvent.setup()
    const { onKey, onBackspace, onClear, onSubmit } = renderKeypad()

    await user.click(screen.getByRole('button', { name: 'clear' }))
    await user.click(screen.getByRole('button', { name: 'backspace' }))
    await user.click(screen.getByRole('button', { name: 'equals' }))

    expect(onClear).toHaveBeenCalledTimes(1)
    expect(onBackspace).toHaveBeenCalledTimes(1)
    expect(onSubmit).toHaveBeenCalledTimes(1)
    expect(onKey).not.toHaveBeenCalled()
  })

  it('disables = while committing and nothing else', () => {
    renderKeypad(true)

    expect(screen.getByRole('button', { name: 'equals' })).toBeDisabled()
    expect(
      screen.getAllByRole('button').filter((button) => button.hasAttribute('disabled')),
    ).toHaveLength(1)
  })

  it('does not take focus on mouse down', async () => {
    const user = userEvent.setup()
    render(
      <>
        <input aria-label="Expression" />
        <Keypad onKey={vi.fn()} onBackspace={vi.fn()} onClear={vi.fn()} committing={false} />
      </>,
    )
    const input = screen.getByLabelText('Expression')
    input.focus()

    await user.click(screen.getByRole('button', { name: '7' }))

    expect(document.activeElement).toBe(input)
  })

  it('is reachable with Tab in order', async () => {
    const user = userEvent.setup()
    renderKeypad()

    await user.tab()
    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'clear' }))
    await user.tab()
    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'backspace' }))
  })
})
