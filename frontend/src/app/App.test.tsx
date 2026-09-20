/// <reference types="node" />
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { renderWithProviders } from '../test/render'
import { App } from './App'

const html = readFileSync(resolve(import.meta.dirname, '../../index.html'), 'utf8')

describe('<App />', () => {
  it('renders the page landmarks and the calculator under the "Calculator" title (UI-1)', () => {
    renderWithProviders(<App />)

    expect(screen.getByRole('banner')).toBeInTheDocument()
    expect(screen.getByRole('main')).toBeInTheDocument()
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Calculator')
    expect(screen.getByLabelText('Expression')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'equals' })).toBeInTheDocument()
  })

  it('ships a document title of "Calculator" (UI-1)', () => {
    expect(html).toContain('<title>Calculator</title>')
  })
})
