import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { EVALUATE_URL, problemResponse, validationFailed } from '../../test/msw/handlers'
import { server } from '../../test/msw/server'
import { renderWithProviders } from '../../test/render'
import { Calculator } from './Calculator'

const UNAVAILABLE = 'The calculator service is unavailable. Try again.'

const input = () => screen.getByLabelText<HTMLInputElement>('Expression')
const output = () => screen.getByRole('status', { name: 'Result' })
/** The polite message region below the result (the only unnamed status region). */
const statusText = () => screen.getByRole('status', { name: '' })
const key = (name: string) => screen.getByRole('button', { name })
const historyRegion = () => screen.getByRole('region', { name: 'History' })

/** History shows its heading and placeholder, and no entries, until the first commit. */
function expectEmptyHistory() {
  expect(within(historyRegion()).getByText('Your calculations will appear here')).toBeVisible()
  expect(within(historyRegion()).queryByRole('list')).not.toBeInTheDocument()
  expect(within(historyRegion()).queryByRole('button')).not.toBeInTheDocument()
}

/** Counts requests and records their expressions, keeping the default fake answers. */
function trackRequests() {
  const sent: string[] = []
  server.use(
    http.post(EVALUATE_URL, async ({ request }) => {
      const body = (await request.clone().json()) as { expression: string }
      sent.push(body.expression)
    }),
  )
  return sent
}

/** A handler whose responses the test releases by hand. */
function deferredResponses() {
  const pending: ((response: { expression: string; result: string }) => void)[] = []
  server.use(
    http.post(EVALUATE_URL, async () => {
      const response = await new Promise<{ expression: string; result: string }>((resolve) => {
        pending.push(resolve)
      })
      return HttpResponse.json(response)
    }),
  )
  return pending
}

/** Asserts the whole result text, not a substring: "4" must not be satisfied by "14". */
async function expectResult(text: string) {
  await waitFor(() => {
    expect(output().textContent).toBe(text)
  })
}

function stubPointer(coarse: boolean) {
  const list = (query: string): MediaQueryList => ({
    matches: coarse && query === '(pointer: coarse)',
    media: query,
    onchange: null,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
    addListener: () => undefined,
    removeListener: () => undefined,
    dispatchEvent: () => false,
  })
  vi.stubGlobal('matchMedia', list)
}

describe('<Calculator />', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  describe('input (FR-9.1, UI-2, UI-3)', () => {
    it('renders one labelled single-line text input with the spec attributes', () => {
      renderWithProviders(<Calculator />)

      expect(screen.getAllByRole('textbox')).toEqual([input()])
      expect(input()).toHaveAttribute('type', 'text')
      expect(input()).toHaveAttribute('maxlength', '1024')
      expect(input()).toHaveAttribute('autocomplete', 'off')
      expect(input()).toHaveAttribute('spellcheck', 'false')
      expect(input()).not.toHaveAttribute('aria-invalid')
      expect(input()).not.toHaveAttribute('aria-describedby')
    })

    it('renders a polite, focusable result region that starts blank', () => {
      renderWithProviders(<Calculator />)

      expect(output().tagName).toBe('OUTPUT')
      expect(output()).toHaveAttribute('aria-live', 'polite')
      expect(output()).toHaveAttribute('tabindex', '0')
      expect(output()).toBeEmptyDOMElement()
      expect(statusText()).toBeEmptyDOMElement()
      expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    })

    it('uses inputMode "none" on coarse-pointer devices', () => {
      stubPointer(true)
      renderWithProviders(<Calculator />)

      expect(input()).toHaveAttribute('inputmode', 'none')
    })

    it('uses inputMode "text" with a fine pointer', () => {
      stubPointer(false)
      renderWithProviders(<Calculator />)

      expect(input()).toHaveAttribute('inputmode', 'text')
    })

    it('uses inputMode "text" when matchMedia is unavailable', () => {
      renderWithProviders(<Calculator />)

      expect(input()).toHaveAttribute('inputmode', 'text')
    })

    it('shows a 118-character result in full', async () => {
      const result = `-${'9'.repeat(100)}.${'9'.repeat(16)}`
      expect(result).toHaveLength(118)
      server.use(
        http.post(EVALUATE_URL, () => HttpResponse.json({ expression: '10^100-1', result })),
      )
      const user = userEvent.setup()
      renderWithProviders(<Calculator />)

      await user.type(input(), '10^100-1')

      await expectResult(result)
    })
  })

  describe('live result (FR-9.2, FR-9.4, FR-9.7)', () => {
    it('shows the live result after typing', async () => {
      const sent = trackRequests()
      const user = userEvent.setup()
      renderWithProviders(<Calculator />)

      await user.type(input(), '2+2')

      await expectResult('4')
      expect(sent).toEqual(['2+2'])
      expect(statusText()).toBeEmptyDOMElement()
    })

    it.each(['(', '-', 'sqrt('])('shows a blank result and no message for %j', async (text) => {
      const user = userEvent.setup()
      renderWithProviders(<Calculator />)
      await user.type(input(), '2+2')
      await expectResult('4')

      await user.clear(input())
      await user.type(input(), text)

      await waitFor(() => {
        expect(output()).toBeEmptyDOMElement()
      })
      await waitFor(() => {
        expect(statusText()).toBeEmptyDOMElement()
      })
      expect(screen.queryByRole('alert')).not.toBeInTheDocument()
      expect(input()).not.toHaveAttribute('aria-invalid')
    })

    it('blanks the result when the input is emptied', async () => {
      const user = userEvent.setup()
      renderWithProviders(<Calculator />)
      await user.type(input(), '2+2')
      await expectResult('4')

      await user.clear(input())

      expect(output()).toBeEmptyDOMElement()
      expect(statusText()).toBeEmptyDOMElement()
    })

    it('shows "Evaluated as" only when the normalized expression differs from the trimmed input', async () => {
      const user = userEvent.setup()
      renderWithProviders(<Calculator />)

      await user.type(input(), '2*(3+4')
      await expectResult('14')
      expect(screen.getByText('Evaluated as 2*(3+4)')).toBeInTheDocument()

      await user.clear(input())
      await user.type(input(), '  2+2 ')
      await expectResult('4')
      expect(screen.queryByText(/Evaluated as/)).not.toBeInTheDocument()
    })

    it('hides "Evaluated as" on an error and after a commit', async () => {
      const user = userEvent.setup()
      renderWithProviders(<Calculator />)
      await user.type(input(), '2*(3+4')
      await expectResult('14')

      server.use(
        http.post(EVALUATE_URL, () =>
          validationFailed('UNBALANCED_PARENTHESIS', "unbalanced ')' at character 8", 7),
        ),
      )
      await user.type(input(), ')')
      await user.type(input(), ')')
      await waitFor(() => {
        expect(statusText()).toHaveTextContent("unbalanced ')' at character 8")
      })
      expect(screen.queryByText(/Evaluated as/)).not.toBeInTheDocument()

      server.resetHandlers()
      await user.clear(input())
      await user.type(input(), '2*(3+4')
      await waitFor(() => {
        expect(screen.getByText('Evaluated as 2*(3+4)')).toBeInTheDocument()
      })
      await user.keyboard('{Enter}')
      await waitFor(() => {
        expect(input()).toHaveValue('14')
      })
      expect(screen.queryByText(/Evaluated as/)).not.toBeInTheDocument()
    })
  })

  describe('keypad (FR-10.2, FR-10.3)', () => {
    it('builds 7*(2+1) from taps and shows 21, keeping focus on the input', async () => {
      const user = userEvent.setup()
      renderWithProviders(<Calculator />)

      for (const name of [
        '7',
        'multiply',
        'open parenthesis',
        '2',
        'add',
        '1',
        'close parenthesis',
      ]) {
        await user.click(key(name))
      }

      expect(input()).toHaveValue('7*(2+1)')
      expect(document.activeElement).toBe(input())
      await expectResult('21')
    })

    it('inserts sqrt( for the square root key', async () => {
      const user = userEvent.setup()
      renderWithProviders(<Calculator />)

      for (const name of ['sqrt, square root', '1', '6', 'close parenthesis']) {
        await user.click(key(name))
      }

      expect(input()).toHaveValue('sqrt(16)')
      await expectResult('4')
    })

    it('appends at the end and moves the caret there even when the caret was elsewhere', async () => {
      const user = userEvent.setup()
      renderWithProviders(<Calculator />)
      await user.type(input(), '12')
      input().setSelectionRange(0, 0)

      await user.click(key('3'))

      expect(input()).toHaveValue('123')
      expect(input().selectionStart).toBe(3)
      expect(input().selectionEnd).toBe(3)
      expect(document.activeElement).toBe(input())
    })
  })

  describe('commit (FR-11)', () => {
    it('replaces the expression with the result, records history and continues from it (FR-11.1)', async () => {
      const user = userEvent.setup()
      renderWithProviders(<Calculator />)

      await user.type(input(), '2+2')
      await user.keyboard('{Enter}')

      await waitFor(() => {
        expect(input()).toHaveValue('4')
      })
      expect(screen.getByRole('button', { name: '2+2 = 4' })).toBeInTheDocument()
      expect(output()).toBeEmptyDOMElement()

      await user.type(input(), '*3')
      await user.keyboard('{Enter}')

      await waitFor(() => {
        expect(input()).toHaveValue('12')
      })
      const history = screen.getByRole('region', { name: 'History' })
      expect(
        within(history)
          .getAllByRole('button')
          .map((b) => b.textContent),
      ).toEqual(['4*3 = 12', '2+2 = 4'])
    })

    it('commits with the = button just like Enter and keeps focus on the input (FR-11.10)', async () => {
      const user = userEvent.setup()
      renderWithProviders(<Calculator />)
      await user.type(input(), '2+2')

      await user.click(key('equals'))

      await waitFor(() => {
        expect(input()).toHaveValue('4')
      })
      expect(document.activeElement).toBe(input())
      expect(input().selectionStart).toBe(1)
    })

    it('records the normalized expression (FR-11.3) and leaves the result blank until the next edit (FR-11.7, FR-9.5)', async () => {
      const sent = trackRequests()
      const user = userEvent.setup()
      renderWithProviders(<Calculator />)

      await user.type(input(), '2*(3+4')
      await user.keyboard('{Enter}')

      await waitFor(() => {
        expect(input()).toHaveValue('14')
      })
      expect(screen.getByRole('button', { name: '2*(3+4) = 14' })).toBeInTheDocument()
      expect(output()).toBeEmptyDOMElement()
      expect(statusText()).toBeEmptyDOMElement()
      const afterCommit = sent.length

      // The next request is the one for the edit, so nothing was sent for the commit itself.
      await user.type(input(), '*3')
      await waitFor(() => {
        expect(sent).toHaveLength(afterCommit + 1)
      })
      expect(sent.at(-1)).toBe('14*3')
    })

    it('shows the arithmetic error as an alert, keeps the input and marks it invalid (FR-11.2, FR-13.5, UI-5)', async () => {
      const user = userEvent.setup()
      renderWithProviders(<Calculator />)

      await user.type(input(), '1/0')
      await user.keyboard('{Enter}')

      const alert = await screen.findByRole('alert')
      expect(alert).toHaveTextContent('Cannot divide by zero.')
      expect(input()).toHaveValue('1/0')
      expect(input()).toHaveAttribute('aria-invalid', 'true')
      expect(input()).toHaveAttribute('aria-describedby', alert.id)
      expect(output()).toBeEmptyDOMElement()
      expect(document.activeElement).toBe(input())
      expectEmptyHistory()
    })

    it('shows a committed validation error verbatim as an alert (FR-13.2)', async () => {
      const user = userEvent.setup()
      renderWithProviders(<Calculator />)

      await user.type(input(), '2+3)')
      await user.keyboard('{Enter}')

      expect(await screen.findByRole('alert')).toHaveTextContent("unbalanced ')' at character 4")
      expect(input()).toHaveAttribute('aria-invalid', 'true')
    })

    it.each(['', '   '])('does nothing on Enter with %j (FR-11.5)', async (text) => {
      const sent = trackRequests()
      const user = userEvent.setup()
      renderWithProviders(<Calculator />)

      if (text !== '') await user.type(input(), text)
      await user.keyboard('{Enter}')
      expectEmptyHistory()
      expect(screen.queryByRole('alert')).not.toBeInTheDocument()

      // The first request ever sent is the one for the next edit, not for the Enter.
      await user.type(input(), '1')
      await waitFor(() => {
        expect(sent).toEqual([`${text}1`])
      })
      expectEmptyHistory()
    })

    it('treats a committed EMPTY as a blank result with no alert and no history (FR-11.6)', async () => {
      const sent = trackRequests()
      const user = userEvent.setup()
      renderWithProviders(<Calculator />)

      await user.type(input(), '(')
      await waitFor(() => {
        expect(sent).toEqual(['('])
      })
      await user.keyboard('{Enter}')
      await waitFor(() => {
        expect(sent).toEqual(['(', '('])
      })
      await waitFor(() => {
        expect(key('equals')).toBeEnabled()
      })

      expect(input()).toHaveValue('(')
      expect(output()).toBeEmptyDOMElement()
      expect(screen.queryByRole('alert')).not.toBeInTheDocument()
      expect(input()).not.toHaveAttribute('aria-invalid')
      expectEmptyHistory()
    })

    it('re-enters a negative result as (-5), stores -5 in history and squares it to 25 (FR-11.8)', async () => {
      const user = userEvent.setup()
      renderWithProviders(<Calculator />)

      await user.type(input(), '2-7')
      await user.keyboard('{Enter}')
      await waitFor(() => {
        expect(input()).toHaveValue('(-5)')
      })
      expect(screen.getByRole('button', { name: '2-7 = -5' })).toBeInTheDocument()

      await user.type(input(), '^2')
      await user.keyboard('{Enter}')

      await waitFor(() => {
        expect(input()).toHaveValue('25')
      })
    })

    it('appends every commit, duplicates included (FR-11.9)', async () => {
      const user = userEvent.setup()
      renderWithProviders(<Calculator />)

      await user.type(input(), '4')
      await user.keyboard('{Enter}')
      await waitFor(() => {
        expect(screen.getByRole('button', { name: '4 = 4' })).toBeInTheDocument()
      })
      await user.keyboard('{Enter}')
      await waitFor(() => {
        expect(screen.getAllByRole('button', { name: '4 = 4' })).toHaveLength(2)
      })
    })

    it('disables = and ignores Enter while a commit is in flight (FR-11.4)', async () => {
      const pending = deferredResponses()
      const user = userEvent.setup()
      renderWithProviders(<Calculator />)

      await user.type(input(), '2+2')
      await user.keyboard('{Enter}')
      await waitFor(() => {
        expect(key('equals')).toBeDisabled()
      })
      await user.keyboard('{Enter}')
      await user.click(key('equals'))
      expect(pending.length).toBeGreaterThanOrEqual(1)
      const requests = pending.length

      pending.forEach((resolve) => {
        resolve({ expression: '2+2', result: '4' })
      })
      await waitFor(() => {
        expect(input()).toHaveValue('4')
      })
      expect(key('equals')).toBeEnabled()
      expect(pending).toHaveLength(requests)
      expect(screen.getAllByRole('button', { name: '2+2 = 4' })).toHaveLength(1)
    })
  })

  describe('clear and backspace (FR-12)', () => {
    it('removes the last character with the backspace key (FR-12.1)', async () => {
      const user = userEvent.setup()
      renderWithProviders(<Calculator />)
      await user.type(input(), '12+3')

      await user.click(key('backspace'))

      expect(input()).toHaveValue('12+')
      expect(document.activeElement).toBe(input())
    })

    it('keeps the native caret behaviour for the keyboard Backspace (UI-4)', async () => {
      const user = userEvent.setup()
      renderWithProviders(<Calculator />)
      await user.type(input(), '12+3')
      input().setSelectionRange(3, 3)

      await user.keyboard('{Backspace}')

      expect(input()).toHaveValue('123')
      expect(input().selectionStart).toBe(2)
    })

    it('clears input, result and alert with Escape, aborts the request and keeps focus and history (FR-12.2)', async () => {
      const user = userEvent.setup()
      renderWithProviders(<Calculator />)
      await user.type(input(), '2+2')
      await user.keyboard('{Enter}')
      await waitFor(() => {
        expect(input()).toHaveValue('4')
      })
      await user.clear(input())
      await user.type(input(), '1/0')
      await user.keyboard('{Enter}')
      await screen.findByRole('alert')

      const pending = deferredResponses()
      await user.type(input(), '+')
      await waitFor(() => {
        expect(pending).toHaveLength(1)
      })

      await user.keyboard('{Escape}')

      expect(input()).toHaveValue('')
      expect(output()).toBeEmptyDOMElement()
      expect(statusText()).toBeEmptyDOMElement()
      expect(screen.queryByRole('alert')).not.toBeInTheDocument()
      expect(input()).not.toHaveAttribute('aria-invalid')
      expect(document.activeElement).toBe(input())
      expect(screen.getByRole('button', { name: '2+2 = 4' })).toBeInTheDocument()

      pending[0]?.({ expression: '1/0+', result: '99' })
      // The stale response resolves first, yet the next result shown is the fresh one.
      server.resetHandlers()
      await user.type(input(), '2+2')
      await expectResult('4')
      expect(screen.queryByText('99')).not.toBeInTheDocument()
    })

    it('leaves the expression alone when Escape is part of an IME composition', async () => {
      const user = userEvent.setup()
      renderWithProviders(<Calculator />)
      await user.type(input(), '2+2')
      await expectResult('4')

      fireEvent.keyDown(input(), { key: 'Escape', isComposing: true })

      expect(input()).toHaveValue('2+2')
      expect(output()).toHaveTextContent('4')

      fireEvent.keyDown(input(), { key: 'Escape' })

      expect(input()).toHaveValue('')
      expect(output()).toBeEmptyDOMElement()
    })

    it('clears the same way with the C key', async () => {
      const user = userEvent.setup()
      renderWithProviders(<Calculator />)
      await user.type(input(), '1/0')
      await user.keyboard('{Enter}')
      await screen.findByRole('alert')

      await user.click(key('clear'))

      expect(input()).toHaveValue('')
      expect(screen.queryByRole('alert')).not.toBeInTheDocument()
      expect(document.activeElement).toBe(input())
    })
  })

  describe('errors (FR-13)', () => {
    beforeEach(() => {
      // Unexpected failures are reported for support; keep the test output quiet.
      vi.spyOn(console, 'warn').mockImplementation(() => undefined)
    })

    it.each([
      { name: 'a network failure', respond: () => HttpResponse.error() },
      { name: 'a 500', respond: () => problemResponse(500, 'INTERNAL_ERROR') },
      { name: 'a 503 TIMEOUT', respond: () => problemResponse(503, 'TIMEOUT') },
    ])(
      'shows the network message as status while typing and as an alert on commit for $name (FR-13.1)',
      async ({ respond }) => {
        server.use(http.post(EVALUATE_URL, respond))
        const user = userEvent.setup()
        renderWithProviders(<Calculator />)

        await user.type(input(), '2+2')
        await waitFor(() => {
          expect(statusText()).toHaveTextContent(UNAVAILABLE)
        })
        expect(screen.queryByRole('alert')).not.toBeInTheDocument()
        expect(input()).not.toHaveAttribute('aria-invalid')

        await user.keyboard('{Enter}')

        expect(await screen.findByRole('alert')).toHaveTextContent(UNAVAILABLE)
        expect(input()).toHaveValue('2+2')
        expect(input()).not.toHaveAttribute('aria-invalid')
        expect(input()).not.toHaveAttribute('aria-describedby')
        expect(document.activeElement).toBe(input())
      },
    )

    it('shows the generic message for an unexpected status', async () => {
      server.use(
        http.post(EVALUATE_URL, () => new HttpResponse('<html>too large</html>', { status: 413 })),
      )
      const user = userEvent.setup()
      renderWithProviders(<Calculator />)

      await user.type(input(), '2+2')

      await waitFor(() => {
        expect(statusText()).toHaveTextContent('Something went wrong. Try again.')
      })
    })

    it('shows validation errors as status text without aria-invalid while typing (FR-13.5, D-3)', async () => {
      const user = userEvent.setup()
      renderWithProviders(<Calculator />)

      await user.type(input(), '2+3)')

      await waitFor(() => {
        expect(statusText()).toHaveTextContent("unbalanced ')' at character 4")
      })
      expect(output()).toBeEmptyDOMElement()
      expect(screen.queryByRole('alert')).not.toBeInTheDocument()
      expect(input()).not.toHaveAttribute('aria-invalid')
    })

    it('clears the alert and aria-invalid on the next edit (FR-13.6)', async () => {
      const user = userEvent.setup()
      renderWithProviders(<Calculator />)
      await user.type(input(), '1/0')
      await user.keyboard('{Enter}')
      await screen.findByRole('alert')

      await user.type(input(), '1')

      expect(screen.queryByRole('alert')).not.toBeInTheDocument()
      expect(input()).not.toHaveAttribute('aria-invalid')
      expect(input()).not.toHaveAttribute('aria-describedby')
      expect(input()).toHaveValue('1/01')
    })
  })

  describe('keyboard focus and typing anywhere (UI-7, decision 38)', () => {
    /** Moves focus to the page background, as after a click on empty space. */
    function blurToBody() {
      input().blur()
      expect(document.activeElement).toBe(document.body)
    }

    it('focuses the expression input as soon as it renders', () => {
      renderWithProviders(<Calculator />)

      expect(input()).toHaveFocus()
    })

    it('routes characters typed on the page background to the input and shows the live result', async () => {
      const user = userEvent.setup()
      renderWithProviders(<Calculator />)
      blurToBody()

      await user.keyboard('2+2')

      expect(input()).toHaveValue('2+2')
      expect(input()).toHaveFocus()
      expect(input().selectionStart).toBe(3)
      await expectResult('4')
    })

    it('removes one character with Backspace pressed on the background', async () => {
      const user = userEvent.setup()
      renderWithProviders(<Calculator />)
      await user.type(input(), '12+3')
      blurToBody()

      await user.keyboard('{Backspace}')

      expect(input()).toHaveValue('12+')
      expect(input()).toHaveFocus()
      expect(input().selectionStart).toBe(3)
    })

    it('commits with Enter pressed on the background', async () => {
      const user = userEvent.setup()
      renderWithProviders(<Calculator />)
      await user.type(input(), '2+2')
      blurToBody()

      await user.keyboard('{Enter}')

      await waitFor(() => {
        expect(input()).toHaveValue('4')
      })
      expect(screen.getAllByRole('button', { name: '2+2 = 4' })).toHaveLength(1)
      expect(input()).toHaveFocus()
    })

    it('clears with Escape pressed on the background', async () => {
      const user = userEvent.setup()
      renderWithProviders(<Calculator />)
      await user.type(input(), '2+2')
      await expectResult('4')
      blurToBody()

      await user.keyboard('{Escape}')

      expect(input()).toHaveValue('')
      expect(output()).toBeEmptyDOMElement()
      expect(input()).toHaveFocus()
    })

    it('appends a character typed while a keypad button has focus and moves focus to the input', async () => {
      const user = userEvent.setup()
      renderWithProviders(<Calculator />)
      await user.type(input(), '2')
      key('add').focus()

      await user.keyboard('7')

      expect(input()).toHaveValue('27')
      expect(input()).toHaveFocus()
      expect(input().selectionStart).toBe(2)
    })

    it('commits exactly once for Enter on the focused equals button', async () => {
      const sent = trackRequests()
      const user = userEvent.setup()
      renderWithProviders(<Calculator />)
      await user.type(input(), '2+2')
      await waitFor(() => {
        expect(sent).toEqual(['2+2'])
      })
      key('equals').focus()

      await user.keyboard('{Enter}')

      await waitFor(() => {
        expect(input()).toHaveValue('4')
      })
      expect(screen.getAllByRole('button', { name: '2+2 = 4' })).toHaveLength(1)
      expect(sent).toEqual(['2+2', '2+2'])
      expect(input()).toHaveFocus()
    })

    it('activates a focused history entry with Enter without committing', async () => {
      const sent = trackRequests()
      const user = userEvent.setup()
      renderWithProviders(<Calculator />)
      await user.type(input(), '2+2')
      await user.keyboard('{Enter}')
      await waitFor(() => {
        expect(input()).toHaveValue('4')
      })
      await user.clear(input())
      await user.type(input(), '3*3')
      await user.keyboard('{Enter}')
      await waitFor(() => {
        expect(input()).toHaveValue('9')
      })
      const before = sent.length
      screen.getByRole('button', { name: '2+2 = 4' }).focus()

      await user.keyboard('{Enter}')

      expect(input()).toHaveValue('2+2')
      expect(input()).toHaveFocus()
      await expectResult('4')
      expect(sent.slice(before)).toEqual(['2+2'])
      expect(
        within(screen.getByRole('region', { name: 'History' }))
          .getAllByRole('button')
          .map((b) => b.textContent),
      ).toEqual(['3*3 = 9', '2+2 = 4'])
    })

    it('activates a focused keypad button with Space, appending its token once', async () => {
      const user = userEvent.setup()
      renderWithProviders(<Calculator />)
      key('7').focus()

      await user.keyboard(' ')

      expect(input()).toHaveValue('7')
      expect(input()).toHaveFocus()
    })

    it('routes Space and Enter pressed while the result has focus', async () => {
      const sent = trackRequests()
      const user = userEvent.setup()
      renderWithProviders(<Calculator />)
      await user.type(input(), '2+2')
      await waitFor(() => {
        expect(sent).toEqual(['2+2'])
      })
      output().focus()

      await user.keyboard(' ')

      expect(input()).toHaveValue('2+2 ')
      expect(input()).toHaveFocus()
      expect(input().selectionStart).toBe(4)
      await waitFor(() => {
        expect(sent).toEqual(['2+2', '2+2 '])
      })
      output().focus()

      await user.keyboard('{Enter}')

      await waitFor(() => {
        expect(input()).toHaveValue('4')
      })
      expect(screen.getAllByRole('button', { name: '2+2 = 4' })).toHaveLength(1)
      expect(sent).toEqual(['2+2', '2+2 ', '2+2 '])
      expect(input()).toHaveFocus()
    })

    it('moves focus to the input for a dead key on the background without typing anything', async () => {
      const user = userEvent.setup()
      renderWithProviders(<Calculator />)
      await user.type(input(), '12')
      blurToBody()
      const prevented: boolean[] = []
      const observe = (event: KeyboardEvent) => {
        prevented.push(event.defaultPrevented)
      }
      window.addEventListener('keydown', observe)

      await user.keyboard('{Dead}')

      window.removeEventListener('keydown', observe)
      expect(input()).toHaveValue('12')
      expect(input()).toHaveFocus()
      expect(input().selectionStart).toBe(2)
      expect(prevented).toEqual([false])
    })

    it('keeps the Tab order from the input through the result to the keypad', async () => {
      const user = userEvent.setup()
      renderWithProviders(<Calculator />)
      expect(input()).toHaveFocus()

      await user.tab()
      expect(output()).toHaveFocus()
      await user.tab()
      expect(key('clear')).toHaveFocus()
      await user.tab()
      expect(key('backspace')).toHaveFocus()
    })

    it.each([
      { name: 'Ctrl', keys: '{Control>}c{/Control}' },
      { name: 'Meta', keys: '{Meta>}r{/Meta}' },
      { name: 'Alt', keys: '{Alt>}7{/Alt}' },
    ])('does not intercept $name combinations', async ({ keys }) => {
      const user = userEvent.setup()
      renderWithProviders(<Calculator />)
      await user.type(input(), '2+2')
      blurToBody()
      const prevented: boolean[] = []
      const observe = (event: KeyboardEvent) => {
        prevented.push(event.defaultPrevented)
      }
      window.addEventListener('keydown', observe)

      await user.keyboard(keys)

      window.removeEventListener('keydown', observe)
      expect(input()).toHaveValue('2+2')
      expect(document.activeElement).toBe(document.body)
      expect(prevented.length).toBeGreaterThan(0)
      expect(prevented).not.toContain(true)
    })

    it('ignores a character typed on the background once the expression is full (FR-10.4)', async () => {
      const user = userEvent.setup()
      renderWithProviders(<Calculator />)
      const full = '1'.repeat(1024)
      await user.click(input())
      await user.paste(full)
      expect(input()).toHaveValue(full)
      blurToBody()

      await user.keyboard('2')

      expect(input()).toHaveValue(full)
      expect(input()).toHaveFocus()
    })
  })

  describe('history (FR-14)', () => {
    it('shows a placeholder until the first commit replaces it with the list (decision 41)', async () => {
      const user = userEvent.setup()
      renderWithProviders(<Calculator />)
      expect(
        within(historyRegion()).getByRole('heading', { level: 2, name: 'History' }),
      ).toBeVisible()
      expectEmptyHistory()

      await user.type(input(), '2+2')
      await user.keyboard('{Enter}')
      await waitFor(() => {
        expect(input()).toHaveValue('4')
      })

      expect(
        within(historyRegion()).getByRole('heading', { level: 2, name: 'History' }),
      ).toBeVisible()
      expect(
        within(historyRegion()).queryByText('Your calculations will appear here'),
      ).not.toBeInTheDocument()
      expect(within(within(historyRegion()).getByRole('list')).getAllByRole('button')).toHaveLength(
        1,
      )
      expect(screen.getByRole('button', { name: '2+2 = 4' })).toBeVisible()
    })

    it('lists commits newest first and reloads an entry through the live path (FR-14.1, FR-14.2)', async () => {
      const sent = trackRequests()
      const user = userEvent.setup()
      renderWithProviders(<Calculator />)
      expectEmptyHistory()

      await user.type(input(), '2+2')
      await user.keyboard('{Enter}')
      await waitFor(() => {
        expect(input()).toHaveValue('4')
      })
      await user.clear(input())
      await user.type(input(), '3*3')
      await user.keyboard('{Enter}')
      await waitFor(() => {
        expect(input()).toHaveValue('9')
      })

      const history = screen.getByRole('region', { name: 'History' })
      expect(
        within(history)
          .getAllByRole('button')
          .map((b) => b.textContent),
      ).toEqual(['3*3 = 9', '2+2 = 4'])
      const before = sent.length

      await user.click(screen.getByRole('button', { name: '2+2 = 4' }))

      expect(input()).toHaveValue('2+2')
      expect(input().selectionStart).toBe(3)
      expect(document.activeElement).toBe(input())
      await expectResult('4')
      expect(sent.slice(before)).toEqual(['2+2'])
    })
  })
})
