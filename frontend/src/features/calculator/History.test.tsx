import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { History } from './History'

describe('<History />', () => {
  it('renders the heading and a placeholder while there are no entries (FR-14.1, decision 41)', () => {
    render(<History entries={[]} onActivate={vi.fn()} />)

    const section = screen.getByRole('region', { name: 'History' })
    expect(within(section).getByRole('heading', { level: 2, name: 'History' })).toBeInTheDocument()
    expect(within(section).getByText('Your calculations will appear here')).toBeVisible()
    expect(within(section).queryByRole('list')).not.toBeInTheDocument()
    expect(within(section).queryByRole('button')).not.toBeInTheDocument()
  })

  it('lists entries as "expression = result" buttons in the given order under a heading (FR-14.1)', () => {
    render(
      <History
        entries={[
          { expression: '3*3', result: '9' },
          { expression: '2+2', result: '4' },
        ]}
        onActivate={vi.fn()}
      />,
    )

    const section = screen.getByRole('region', { name: 'History' })
    expect(within(section).getByRole('heading', { level: 2, name: 'History' })).toBeInTheDocument()
    const items = within(section).getAllByRole('listitem')
    expect(items.map((item) => within(item).getByRole('button').textContent)).toEqual([
      '3*3 = 9',
      '2+2 = 4',
    ])
    expect(within(section).getByRole('list')).toBeInTheDocument()
    expect(
      within(section).queryByText('Your calculations will appear here'),
    ).not.toBeInTheDocument()
  })

  it('shows long expressions and results in full (FR-14.5)', () => {
    const result = '1'.repeat(100)
    render(<History entries={[{ expression: '10^100-1', result }]} onActivate={vi.fn()} />)

    expect(screen.getByRole('button', { name: `10^100-1 = ${result}` })).toBeInTheDocument()
  })

  it('reports the index of the activated entry, by mouse and by keyboard', async () => {
    const user = userEvent.setup()
    const onActivate = vi.fn()
    render(
      <History
        entries={[
          { expression: '3*3', result: '9' },
          { expression: '2+2', result: '4' },
        ]}
        onActivate={onActivate}
      />,
    )

    await user.click(screen.getByRole('button', { name: '2+2 = 4' }))
    screen.getByRole('button', { name: '3*3 = 9' }).focus()
    await user.keyboard('{Enter}')

    expect(onActivate.mock.calls).toEqual([[1], [0]])
  })
})
