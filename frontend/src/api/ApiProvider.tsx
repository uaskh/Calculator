import type { ReactNode } from 'react'
import { ApiContext } from './apiContext'
import type { HttpClient } from './client'

interface ApiProviderProps {
  client: HttpClient
  children: ReactNode
}

/** Makes the HTTP client available to hooks; tests pass a client of their choice. */
export function ApiProvider({ client, children }: ApiProviderProps) {
  return <ApiContext value={client}>{children}</ApiContext>
}
