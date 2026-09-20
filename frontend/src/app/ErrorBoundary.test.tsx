import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { ErrorBoundary } from './ErrorBoundary'

let shouldThrow = false

function Flaky() {
  if (shouldThrow) throw new Error('render failed')
  return <p>content</p>
}

describe('<ErrorBoundary />', () => {
  it('shows a fallback and recovers on retry', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => undefined)
    shouldThrow = true
    render(
      <ErrorBoundary>
        <Flaky />
      </ErrorBoundary>,
    )

    expect(screen.getByRole('alert')).toHaveTextContent('Something went wrong')

    shouldThrow = false
    await userEvent.setup().click(screen.getByRole('button', { name: 'Try again' }))

    expect(screen.getByText('content')).toBeInTheDocument()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })
})
