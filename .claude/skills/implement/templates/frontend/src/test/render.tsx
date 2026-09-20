import { render, type RenderOptions } from '@testing-library/react'
import type { ReactElement, ReactNode } from 'react'
import { ApiProvider } from '../api/ApiProvider'
import { createHttpClient, type HttpClient } from '../api/client'

interface ProviderOptions extends Omit<RenderOptions, 'wrapper'> {
  /** Defaults to the real client, answered by the MSW handlers. */
  client?: HttpClient
}

/** Renders ui inside the same providers the application uses. */
export function renderWithProviders(ui: ReactElement, options: ProviderOptions = {}) {
  const { client = createHttpClient(), ...renderOptions } = options
  function Providers({ children }: { children: ReactNode }) {
    return <ApiProvider client={client}>{children}</ApiProvider>
  }
  return render(ui, { wrapper: Providers, ...renderOptions })
}
