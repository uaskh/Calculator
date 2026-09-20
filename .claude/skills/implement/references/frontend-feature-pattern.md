# Frontend feature pattern

A complete vertical slice for an illustrative "widgets" feature (create a widget, list
widgets in a chosen order) that talks to the backend example in
[backend-feature-pattern.md](backend-feature-pattern.md). The names are placeholders: copy
the shape, not the domain. The code type-checks against the scaffold under strict settings;
run its tests in the real project.

## Recipe

1. **API module** (`src/api/<resource>.ts`): DTO types mirroring the OpenAPI schema, parsers
   that turn `unknown` into those types (throwing on bad shapes), allowed query values as a
   `const` list with a type guard, and endpoint functions that take the `HttpClient` and
   optional `RequestOptions`.
2. **Model** (`src/features/<feature>/model.ts`): pure functions and reducers for
   validation, state transitions and user-facing messages. Unit-test them exhaustively.
3. **Hooks**: an action hook (`use<Action>.ts`) wires the API to a reducer, cancels the
   request in flight on unmount and when superseded, and ignores aborted requests. A data
   hook (`use<Things>.ts`) fetches in an effect that aborts on cleanup (safe under
   StrictMode), stores each result with the key of its request so stale results are never
   shown, and derives "loading" instead of setting it inside the effect.
4. **Components**: semantic, labelled, keyboard-friendly markup; errors in `role="alert"`
   linked with `aria-describedby`; results and list summaries in a `role="status"` live
   region that is always rendered; explicit loading, empty, error (with retry) and success
   states; disabled submit while pending; input kept on failure. Styles in a CSS module
   using tokens (`--color-control-border` for form controls).
5. **Component tests** with MSW handlers for success, empty results, validation errors from
   the server, network failures, retries and slow responses, plus client-side validation.
6. **Default MSW handlers** for the happy path in `src/test/msw/handlers.ts`, one per
   operation, so other tests can render the feature.
7. **Browser journeys** in `e2e/`: keyboard use, a simulated server failure, and data that
   is unique per test so parallel projects and retries never collide.
8. Compose the feature in `src/app/App.tsx`; siblings coordinate through props (here a
   refresh key), not shared global state.

## Files

### `src/api/widgets.ts`

```ts
import type { HttpClient, RequestOptions } from './client'
import { asRecord, readString } from './validate'

export interface Widget {
  id: string
  name: string
  /** ISO 8601 timestamp. */
  createdAt: string
}

export interface CreateWidgetInput {
  name: string
}

/** Sort orders the API accepts (GET /v1/widgets?sort=); the first one is the default. */
export const WIDGET_SORTS = ['created', 'name'] as const
export type WidgetSort = (typeof WIDGET_SORTS)[number]

export function isWidgetSort(value: string): value is WidgetSort {
  return (WIDGET_SORTS as readonly string[]).includes(value)
}

export function parseWidget(data: unknown): Widget {
  const record = asRecord(data, 'widget')
  return {
    id: readString(record, 'id'),
    name: readString(record, 'name'),
    createdAt: readString(record, 'createdAt'),
  }
}

export function parseWidgetList(data: unknown): Widget[] {
  const items: unknown = asRecord(data, 'widget list').items
  if (!Array.isArray(items)) throw new TypeError('items is not an array')
  return items.map((item: unknown) => parseWidget(item))
}

export function createWidget(
  client: HttpClient,
  input: CreateWidgetInput,
  options?: RequestOptions,
): Promise<Widget> {
  return client.post('/v1/widgets', input, parseWidget, options)
}

export function listWidgets(
  client: HttpClient,
  sort: WidgetSort,
  options?: RequestOptions,
): Promise<Widget[]> {
  return client.get(`/v1/widgets?sort=${encodeURIComponent(sort)}`, parseWidgetList, options)
}
```

### `src/features/widgets/model.ts`

```ts
import type { ApiError } from '../../api/errors'
import type { Widget, WidgetSort } from '../../api/widgets'

export const NAME_MAX_LENGTH = 64

/** Mirrors the server rule for instant feedback; the server stays authoritative. */
export function validateName(raw: string): string | null {
  const name = raw.trim()
  if (name === '') return 'Enter a name.'
  if ([...name].length > NAME_MAX_LENGTH) return `Use at most ${NAME_MAX_LENGTH} characters.`
  return null
}

export type SubmitState =
  | { status: 'idle' }
  | { status: 'submitting' }
  | { status: 'succeeded'; widget: Widget }
  | { status: 'failed'; error: ApiError }

export type SubmitEvent =
  | { type: 'submitted' }
  | { type: 'succeeded'; widget: Widget }
  | { type: 'failed'; error: ApiError }
  | { type: 'edited' }

export function submitReducer(state: SubmitState, event: SubmitEvent): SubmitState {
  switch (event.type) {
    case 'submitted':
      return { status: 'submitting' }
    case 'succeeded':
      return { status: 'succeeded', widget: event.widget }
    case 'failed':
      return { status: 'failed', error: event.error }
    case 'edited':
      return state.status === 'failed' ? { status: 'idle' } : state
  }
}

/** User-facing copy for a failed submission; branches on kind and code, never on text. */
export function errorMessage(error: ApiError): string {
  switch (error.kind) {
    case 'network':
      return 'Could not reach the server. Check your connection and try again.'
    case 'timeout':
      return 'The server took too long to respond. Try again.'
    case 'http': {
      const field = error.fieldErrors().name
      if (field && (error.code === 'VALIDATION_FAILED' || error.code === 'INVALID_NAME')) {
        return `Name ${field}.`
      }
      return 'Something went wrong on our side. Try again later.'
    }
    default:
      return 'Something went wrong. Try again.'
  }
}

/** Labels for the sort control: a new sort order does not compile until it has one. */
export const SORT_LABELS: Record<WidgetSort, string> = {
  created: 'Oldest first',
  name: 'Name',
}

/** User-facing copy for a list that could not be loaded. */
export function loadErrorMessage(error: ApiError): string {
  switch (error.kind) {
    case 'network':
      return 'Could not reach the server. Check your connection and try again.'
    case 'timeout':
      return 'The server took too long to respond. Try again.'
    default:
      return 'The widgets could not be loaded. Try again.'
  }
}

/** Text for the list's live region. */
export function listSummary(count: number): string {
  if (count === 0) return 'No widgets yet.'
  return count === 1 ? '1 widget' : `${count} widgets`
}
```

### `src/features/widgets/model.test.ts`

```ts
import { describe, expect, it } from 'vitest'
import { ApiError } from '../../api/errors'
import { WIDGET_SORTS } from '../../api/widgets'
import {
  errorMessage,
  listSummary,
  loadErrorMessage,
  NAME_MAX_LENGTH,
  SORT_LABELS,
  submitReducer,
  validateName,
  type SubmitState,
} from './model'

describe('validateName', () => {
  it.each([
    { raw: '', expected: 'Enter a name.' },
    { raw: '   ', expected: 'Enter a name.' },
    {
      raw: 'a'.repeat(NAME_MAX_LENGTH + 1),
      expected: `Use at most ${NAME_MAX_LENGTH} characters.`,
    },
    { raw: 'é'.repeat(NAME_MAX_LENGTH), expected: null },
    { raw: ' lamp ', expected: null },
  ])('returns $expected for "$raw"', ({ raw, expected }) => {
    expect(validateName(raw)).toBe(expected)
  })
})

describe('submitReducer', () => {
  const widget = { id: 'w-1', name: 'lamp', createdAt: '2026-01-02T03:04:05Z' }
  const failed: SubmitState = { status: 'failed', error: new ApiError('network', 'down') }

  it('moves through a successful submission', () => {
    const submitting = submitReducer({ status: 'idle' }, { type: 'submitted' })
    expect(submitting).toEqual({ status: 'submitting' })
    expect(submitReducer(submitting, { type: 'succeeded', widget })).toEqual({
      status: 'succeeded',
      widget,
    })
  })

  it('clears a failure when the user edits, but keeps other states', () => {
    expect(submitReducer(failed, { type: 'edited' })).toEqual({ status: 'idle' })
    expect(submitReducer({ status: 'submitting' }, { type: 'edited' })).toEqual({
      status: 'submitting',
    })
  })
})

describe('errorMessage', () => {
  const problem = (code: string, errors: { field: string; code: string; message: string }[] = []) =>
    new ApiError('http', 'failed', {
      status: 422,
      problem: { type: 'about:blank', title: 'Unprocessable Content', status: 422, code, errors },
    })

  it.each([
    {
      error: new ApiError('network', 'x'),
      expected: 'Could not reach the server. Check your connection and try again.',
    },
    {
      error: new ApiError('timeout', 'x'),
      expected: 'The server took too long to respond. Try again.',
    },
    { error: new ApiError('invalid-response', 'x'), expected: 'Something went wrong. Try again.' },
    {
      error: problem('INVALID_NAME', [
        { field: 'name', code: 'TAKEN', message: 'is already taken' },
      ]),
      expected: 'Name is already taken.',
    },
    {
      error: problem('INVALID_NAME'),
      expected: 'Something went wrong on our side. Try again later.',
    },
    {
      error: problem('INTERNAL_ERROR'),
      expected: 'Something went wrong on our side. Try again later.',
    },
  ])('maps $error.kind/$error.code', ({ error, expected }) => {
    expect(errorMessage(error)).toBe(expected)
  })
})

describe('loadErrorMessage', () => {
  it.each([
    {
      error: new ApiError('network', 'x'),
      expected: 'Could not reach the server. Check your connection and try again.',
    },
    {
      error: new ApiError('timeout', 'x'),
      expected: 'The server took too long to respond. Try again.',
    },
    {
      error: new ApiError('http', 'x', { status: 500 }),
      expected: 'The widgets could not be loaded. Try again.',
    },
  ])('maps $error.kind', ({ error, expected }) => {
    expect(loadErrorMessage(error)).toBe(expected)
  })
})

describe('listSummary', () => {
  it.each([
    { count: 0, expected: 'No widgets yet.' },
    { count: 1, expected: '1 widget' },
    { count: 3, expected: '3 widgets' },
  ])('describes $count widgets', ({ count, expected }) => {
    expect(listSummary(count)).toBe(expected)
  })
})

describe('SORT_LABELS', () => {
  it('labels every sort order the API accepts', () => {
    expect(WIDGET_SORTS.every((sort) => SORT_LABELS[sort].length > 0)).toBe(true)
  })
})
```

### `src/features/widgets/useCreateWidget.ts`

```ts
import { useCallback, useEffect, useReducer, useRef } from 'react'
import { toApiError } from '../../api/errors'
import { useApi } from '../../api/useApi'
import { createWidget, type Widget } from '../../api/widgets'
import { submitReducer } from './model'

/**
 * Submits widget creations and reports each created widget to `onCreated`. A new
 * submission or unmounting cancels the one in flight.
 */
export function useCreateWidget(onCreated?: (widget: Widget) => void) {
  const api = useApi()
  const [state, dispatch] = useReducer(submitReducer, { status: 'idle' })
  const inFlight = useRef<AbortController | null>(null)

  useEffect(() => {
    const pending = inFlight // the ref object is stable; read .current when unmounting
    return () => {
      pending.current?.abort()
    }
  }, [])

  const submit = useCallback(
    async (name: string) => {
      inFlight.current?.abort()
      const controller = new AbortController()
      inFlight.current = controller
      dispatch({ type: 'submitted' })
      let widget: Widget
      try {
        widget = await createWidget(api, { name: name.trim() }, { signal: controller.signal })
      } catch (error) {
        const apiError = toApiError(error)
        if (apiError.kind !== 'aborted') dispatch({ type: 'failed', error: apiError })
        return
      } finally {
        if (inFlight.current === controller) inFlight.current = null
      }
      dispatch({ type: 'succeeded', widget })
      onCreated?.(widget)
    },
    [api, onCreated],
  )

  const edited = useCallback(() => {
    dispatch({ type: 'edited' })
  }, [])

  return { state, submit, edited }
}
```

### `src/features/widgets/CreateWidgetForm.tsx`

```tsx
import { useId, useState, type FormEvent } from 'react'
import type { Widget } from '../../api/widgets'
import styles from './CreateWidgetForm.module.css'
import { errorMessage, validateName } from './model'
import { useCreateWidget } from './useCreateWidget'

interface CreateWidgetFormProps {
  /** Called after the server has created a widget. */
  onCreated?: (widget: Widget) => void
}

export function CreateWidgetForm({ onCreated }: CreateWidgetFormProps) {
  const { state, submit, edited } = useCreateWidget(onCreated)
  const [name, setName] = useState('')
  const [touched, setTouched] = useState(false)
  const inputId = useId()
  const errorId = useId()

  const submitting = state.status === 'submitting'
  const clientError = touched ? validateName(name) : null
  const error = clientError ?? (state.status === 'failed' ? errorMessage(state.error) : null)

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setTouched(true)
    if (submitting || validateName(name) !== null) return
    void submit(name)
  }

  return (
    <form className={styles.form} onSubmit={handleSubmit} noValidate aria-busy={submitting}>
      <label className={styles.label} htmlFor={inputId}>
        Name
      </label>
      <input
        id={inputId}
        className={styles.input}
        name="name"
        autoComplete="off"
        value={name}
        aria-invalid={error ? true : undefined}
        aria-describedby={error ? errorId : undefined}
        onChange={(event) => {
          setName(event.target.value)
          edited()
        }}
        onBlur={() => {
          setTouched(true)
        }}
      />
      {error && (
        <p id={errorId} className={styles.error} role="alert">
          {error}
        </p>
      )}
      <button className={styles.button} type="submit" disabled={submitting}>
        {submitting ? 'Saving…' : 'Create widget'}
      </button>
      <p className={styles.status} role="status">
        {state.status === 'succeeded' ? `Created “${state.widget.name}”.` : ''}
      </p>
    </form>
  )
}
```

### `src/features/widgets/CreateWidgetForm.module.css`

```css
.form {
  display: grid;
  gap: var(--space-2);
  max-width: 28rem;
}

.label {
  font-weight: 600;
}

.input {
  min-height: var(--tap-target);
  padding-inline: var(--space-3);
  border: 1px solid var(--color-control-border);
  border-radius: var(--radius-sm);
  color: var(--color-text);
  background: var(--color-bg);
}

.input[aria-invalid='true'] {
  border-color: var(--color-danger);
}

.error {
  margin: 0;
  color: var(--color-danger);
  font-size: var(--text-sm);
}

.button {
  justify-self: start;
  min-height: var(--tap-target);
  padding-inline: var(--space-4);
  border: 0;
  border-radius: var(--radius-sm);
  color: var(--color-accent-contrast);
  background: var(--color-accent);
  cursor: pointer;
}

.button:disabled {
  opacity: 0.6;
  cursor: progress;
}

.status {
  min-height: 1.5em;
  margin: 0;
  color: var(--color-success);
}
```

### `src/features/widgets/CreateWidgetForm.test.tsx`

```tsx
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { delay, http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { asRecord, readString } from '../../api/validate'
import { server } from '../../test/msw/server'
import { renderWithProviders } from '../../test/render'
import { CreateWidgetForm } from './CreateWidgetForm'

const endpoint = '/api/v1/widgets'
const createdAt = '2026-01-02T03:04:05Z'

function setup(onCreated?: (id: string) => void) {
  const user = userEvent.setup()
  renderWithProviders(
    <CreateWidgetForm
      onCreated={(widget) => {
        onCreated?.(widget.id)
      }}
    />,
  )
  return {
    user,
    nameInput: () => screen.getByLabelText('Name'),
    submitButton: () => screen.getByRole('button', { name: /create widget|saving/i }),
  }
}

describe('<CreateWidgetForm />', () => {
  it('creates a widget, announces it and tells the parent', async () => {
    server.use(
      http.post(endpoint, async ({ request }) => {
        const body = asRecord(await request.json())
        return HttpResponse.json(
          { id: 'w-1', name: readString(body, 'name'), createdAt },
          { status: 201 },
        )
      }),
    )
    const created: string[] = []
    const { user, nameInput, submitButton } = setup((id) => created.push(id))

    await user.type(nameInput(), '  lamp ')
    await user.click(submitButton())

    expect(await screen.findByText('Created “lamp”.')).toBeInTheDocument()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(created).toEqual(['w-1'])
  })

  it('validates before calling the server', async () => {
    const { user, nameInput, submitButton } = setup() // no handler: a request would fail the test

    await user.click(submitButton())

    expect(screen.getByRole('alert')).toHaveTextContent('Enter a name.')
    expect(nameInput()).toHaveAttribute('aria-invalid', 'true')
    expect(nameInput()).toHaveAccessibleDescription('Enter a name.')
  })

  it('shows validation messages from the server', async () => {
    server.use(
      http.post(endpoint, () =>
        HttpResponse.json(
          {
            type: 'about:blank',
            title: 'Bad Request',
            status: 400,
            code: 'VALIDATION_FAILED',
            errors: [
              { field: 'name', code: 'INVALID_LENGTH', message: 'must be 1 to 64 characters' },
            ],
          },
          { status: 400, headers: { 'Content-Type': 'application/problem+json' } },
        ),
      ),
    )
    const { user, nameInput, submitButton } = setup()

    await user.type(nameInput(), 'lamp')
    await user.click(submitButton())

    expect(await screen.findByRole('alert')).toHaveTextContent('Name must be 1 to 64 characters.')
  })

  it('keeps the input and explains network failures', async () => {
    server.use(http.post(endpoint, () => HttpResponse.error()))
    const created: string[] = []
    const { user, nameInput, submitButton } = setup((id) => created.push(id))

    await user.type(nameInput(), 'lamp')
    await user.click(submitButton())

    expect(await screen.findByRole('alert')).toHaveTextContent('Could not reach the server')
    expect(nameInput()).toHaveValue('lamp')
    expect(created).toEqual([])

    await user.type(nameInput(), 's')
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('prevents double submission while saving', async () => {
    let calls = 0
    server.use(
      http.post(endpoint, async () => {
        calls += 1
        await delay('infinite')
        return HttpResponse.json({})
      }),
    )
    const { user, nameInput, submitButton } = setup()

    await user.type(nameInput(), 'lamp')
    await user.click(submitButton())
    await user.click(submitButton())

    expect(submitButton()).toBeDisabled()
    expect(submitButton()).toHaveTextContent('Saving…')
    expect(calls).toBe(1)
  })
})
```

### `src/features/widgets/useWidgets.ts`

```ts
import { useCallback, useEffect, useState } from 'react'
import { toApiError, type ApiError } from '../../api/errors'
import { useApi } from '../../api/useApi'
import { listWidgets, type Widget, type WidgetSort } from '../../api/widgets'

export type WidgetListState =
  | { status: 'loading' }
  | { status: 'loaded'; items: Widget[] }
  | { status: 'failed'; error: ApiError }

interface Settled {
  key: string
  state: WidgetListState
}

/**
 * Loads the widget list in the given order. Changing the order or `refreshKey`, or calling
 * `reload`, starts a new request and cancels the previous one. Results are stored with the
 * key of the request that produced them, so a superseded result is never shown and
 * "loading" is derived instead of set from the effect.
 */
export function useWidgets(sort: WidgetSort, refreshKey = 0) {
  const api = useApi()
  const [attempt, setAttempt] = useState(0)
  const [settled, setSettled] = useState<Settled | null>(null)
  const key = `${sort}|${refreshKey}|${attempt}`

  useEffect(() => {
    const controller = new AbortController()
    listWidgets(api, sort, { signal: controller.signal }).then(
      (items) => {
        setSettled({ key, state: { status: 'loaded', items } })
      },
      (error: unknown) => {
        const apiError = toApiError(error)
        if (apiError.kind !== 'aborted') {
          setSettled({ key, state: { status: 'failed', error: apiError } })
        }
      },
    )
    return () => {
      controller.abort()
    }
  }, [api, key, sort])

  const reload = useCallback(() => {
    setAttempt((count) => count + 1)
  }, [])

  const state: WidgetListState = settled?.key === key ? settled.state : { status: 'loading' }
  return { state, reload }
}
```

### `src/features/widgets/WidgetList.tsx`

```tsx
import { useId, useState } from 'react'
import { isWidgetSort, WIDGET_SORTS, type WidgetSort } from '../../api/widgets'
import { listSummary, loadErrorMessage, SORT_LABELS } from './model'
import { useWidgets } from './useWidgets'
import styles from './WidgetList.module.css'

interface WidgetListProps {
  /** Changing this value reloads the list, for example after a widget was created. */
  refreshKey?: number
}

export function WidgetList({ refreshKey = 0 }: WidgetListProps) {
  const [sort, setSort] = useState<WidgetSort>(WIDGET_SORTS[0])
  const { state, reload } = useWidgets(sort, refreshKey)
  const headingId = useId()
  const sortId = useId()

  return (
    <section className={styles.section} aria-labelledby={headingId}>
      <div className={styles.toolbar}>
        <h2 id={headingId} className={styles.heading}>
          Widgets
        </h2>
        <label className={styles.sortLabel} htmlFor={sortId}>
          Sort by
        </label>
        <select
          id={sortId}
          className={styles.select}
          value={sort}
          onChange={(event) => {
            if (isWidgetSort(event.target.value)) setSort(event.target.value)
          }}
        >
          {WIDGET_SORTS.map((option) => (
            <option key={option} value={option}>
              {SORT_LABELS[option]}
            </option>
          ))}
        </select>
      </div>

      <p className={styles.summary} role="status">
        {state.status === 'loading' && 'Loading widgets…'}
        {state.status === 'loaded' && listSummary(state.items.length)}
      </p>

      {state.status === 'failed' && (
        <div className={styles.error} role="alert">
          <p className={styles.errorText}>{loadErrorMessage(state.error)}</p>
          <button className={styles.retry} type="button" onClick={reload}>
            Try again
          </button>
        </div>
      )}

      {state.status === 'loaded' && state.items.length > 0 && (
        <ul className={styles.list} aria-labelledby={headingId}>
          {state.items.map((widget) => (
            <li key={widget.id} className={styles.item}>
              {widget.name}
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}
```

### `src/features/widgets/WidgetList.module.css`

```css
.section {
  display: grid;
  gap: var(--space-3);
  margin-top: var(--space-8);
}

.toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2) var(--space-3);
}

.heading {
  margin: 0 auto 0 0;
  font-size: var(--text-lg);
}

.sortLabel {
  font-weight: 600;
}

.select {
  min-height: var(--tap-target);
  padding-inline: var(--space-2);
  border: 1px solid var(--color-control-border);
  border-radius: var(--radius-sm);
  color: var(--color-text);
  background: var(--color-bg);
}

.summary {
  min-height: 1.5em;
  margin: 0;
  color: var(--color-text-muted);
}

.error {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2) var(--space-3);
  color: var(--color-danger);
}

.errorText {
  margin: 0;
}

.retry {
  min-height: var(--tap-target);
  padding-inline: var(--space-4);
  border: 1px solid var(--color-control-border);
  border-radius: var(--radius-sm);
  color: var(--color-text);
  background: var(--color-surface);
  cursor: pointer;
}

.list {
  display: grid;
  gap: var(--space-2);
  margin: 0;
  padding-inline-start: var(--space-6);
}

.item {
  overflow-wrap: anywhere;
}
```

### `src/features/widgets/WidgetList.test.tsx`

```tsx
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { server } from '../../test/msw/server'
import { renderWithProviders } from '../../test/render'
import { WidgetList } from './WidgetList'

const endpoint = '/api/v1/widgets'
const widget = (id: string, name: string) => ({ id, name, createdAt: '2026-01-02T03:04:05Z' })

function listedNames() {
  const list = screen.getByRole('list', { name: 'Widgets' })
  return within(list)
    .getAllByRole('listitem')
    .map((item) => item.textContent)
}

describe('<WidgetList />', () => {
  it('loads the widgets in the default order', async () => {
    const requested: (string | null)[] = []
    server.use(
      http.get(endpoint, ({ request }) => {
        requested.push(new URL(request.url).searchParams.get('sort'))
        return HttpResponse.json({ items: [widget('w-1', 'lamp'), widget('w-2', 'desk')] })
      }),
    )
    renderWithProviders(<WidgetList />)

    expect(screen.getByRole('status')).toHaveTextContent('Loading widgets…')
    expect(await screen.findByRole('list', { name: 'Widgets' })).toBeInTheDocument()
    expect(listedNames()).toEqual(['lamp', 'desk'])
    expect(screen.getByRole('status')).toHaveTextContent('2 widgets')
    expect(requested).toEqual(['created'])
  })

  it('says so when there are no widgets', async () => {
    server.use(http.get(endpoint, () => HttpResponse.json({ items: [] })))
    renderWithProviders(<WidgetList />)

    expect(await screen.findByText('No widgets yet.')).toBeInTheDocument()
    expect(screen.queryByRole('list')).not.toBeInTheDocument()
  })

  it('reloads in the order the user picks', async () => {
    server.use(
      http.get(endpoint, ({ request }) => {
        const byName = new URL(request.url).searchParams.get('sort') === 'name'
        const items = [widget('w-1', 'lamp'), widget('w-2', 'desk')]
        return HttpResponse.json({ items: byName ? items.reverse() : items })
      }),
    )
    const user = userEvent.setup()
    renderWithProviders(<WidgetList />)
    await screen.findByRole('list', { name: 'Widgets' })

    await user.selectOptions(screen.getByLabelText('Sort by'), 'name')

    await waitFor(() => {
      expect(listedNames()).toEqual(['desk', 'lamp'])
    })
  })

  it('explains a failure and recovers on retry', async () => {
    let calls = 0
    server.use(
      http.get(endpoint, () => {
        calls += 1
        return calls === 1
          ? HttpResponse.error()
          : HttpResponse.json({ items: [widget('w-1', 'lamp')] })
      }),
    )
    const user = userEvent.setup()
    renderWithProviders(<WidgetList />)

    expect(await screen.findByRole('alert')).toHaveTextContent('Could not reach the server')
    await user.click(screen.getByRole('button', { name: 'Try again' }))

    expect(await screen.findByRole('list', { name: 'Widgets' })).toHaveTextContent('lamp')
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('reloads when refreshKey changes', async () => {
    let calls = 0
    server.use(
      http.get(endpoint, () => {
        calls += 1
        return HttpResponse.json({ items: calls === 1 ? [] : [widget('w-1', 'lamp')] })
      }),
    )
    const { rerender } = renderWithProviders(<WidgetList refreshKey={0} />)
    await screen.findByText('No widgets yet.')

    rerender(<WidgetList refreshKey={1} />)

    expect(await screen.findByRole('list', { name: 'Widgets' })).toHaveTextContent('lamp')
  })
})
```

### `e2e/widgets.spec.ts`

```ts
import { expect, test } from '@playwright/test'

/** Unique per project, test and retry, so tests that share one backend never collide. */
function uniqueName(prefix: string): string {
  const info = test.info()
  return `${prefix} ${info.project.name}-${info.testId}-${info.retry}`
}

test.describe('widgets', () => {
  test('creates a widget with the keyboard and lists it', async ({ page }) => {
    const name = uniqueName('Desk lamp')
    await page.goto('/')

    await page.getByLabel('Name', { exact: true }).fill(name)
    await page.keyboard.press('Enter')

    await expect(page.getByRole('status').filter({ hasText: 'Created' })).toHaveText(
      `Created “${name}”.`,
    )
    await expect(page.getByRole('list', { name: 'Widgets' }).getByText(name)).toBeVisible()
  })

  test('explains invalid input', async ({ page }) => {
    await page.goto('/')

    await page.getByRole('button', { name: 'Create widget' }).click()

    await expect(page.getByRole('alert')).toHaveText('Enter a name.')
    await expect(page.getByLabel('Name', { exact: true })).toHaveAttribute('aria-invalid', 'true')
  })

  test('recovers after a server failure', async ({ page }) => {
    const name = uniqueName('Desk lamp')
    await page.route('**/api/v1/widgets', (route) => {
      if (route.request().method() !== 'POST') return route.fallback()
      return route.fulfill({
        status: 500,
        contentType: 'application/problem+json',
        body: JSON.stringify({
          type: 'about:blank',
          title: 'Internal Server Error',
          status: 500,
          code: 'INTERNAL_ERROR',
        }),
      })
    })
    await page.goto('/')
    await page.getByLabel('Name', { exact: true }).fill(name)
    await page.getByRole('button', { name: 'Create widget' }).click()
    await expect(page.getByRole('alert')).toContainText('Something went wrong')

    await page.unroute('**/api/v1/widgets')
    await page.getByRole('button', { name: 'Create widget' }).click()

    await expect(page.getByRole('status').filter({ hasText: 'Created' })).toHaveText(
      `Created “${name}”.`,
    )
  })

  test('sorts the list by name', async ({ page, request }) => {
    const suffix = uniqueName('')
    for (const name of [`zeta${suffix}`, `Alpha${suffix}`]) {
      const response = await request.post('/api/v1/widgets', { data: { name } })
      expect(response.status()).toBe(201)
    }
    await page.goto('/')

    await page.getByLabel('Sort by').selectOption('name')

    const items = page
      .getByRole('list', { name: 'Widgets' })
      .getByRole('listitem')
      .filter({ hasText: suffix })
    await expect(items).toHaveText([`Alpha${suffix}`, `zeta${suffix}`])
  })
})
```

### `src/app/App.tsx` (composition)

```tsx
import { useState } from 'react'
import { CreateWidgetForm } from '../features/widgets/CreateWidgetForm'
import { WidgetList } from '../features/widgets/WidgetList'
// …
export function App() {
  const [createdCount, setCreatedCount] = useState(0)
  // …
      <main className={styles.main}>
        <ErrorBoundary>
          <CreateWidgetForm
            onCreated={() => {
              setCreatedCount((count) => count + 1)
            }}
          />
          <WidgetList refreshKey={createdCount} />
        </ErrorBoundary>
      </main>
```

### `src/test/msw/handlers.ts (default happy paths)`

```ts
import { http, HttpResponse, type RequestHandler } from 'msw'

const createdAt = '2026-01-02T03:04:05Z'

export const handlers: RequestHandler[] = [
  http.get('/api/v1/widgets', () => HttpResponse.json({ items: [] })),
  http.post('/api/v1/widgets', () =>
    HttpResponse.json({ id: 'w-1', name: 'lamp', createdAt }, { status: 201 }),
  ),
]
```

## Checklist for every feature

- Loading, success, empty and error states are visible and tested.
- Every input has a label; errors are announced and linked; focus order is logical.
- Works at 320–375 px wide without horizontal scrolling; touch targets ≥ 44 px.
- No request is left running after unmount; stale responses are ignored; double
  submission is impossible.
- Copy comes from the spec or the plan's UI section, not from guesswork.
