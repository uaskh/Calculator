import { useContext } from 'react'
import { ApiContext } from './apiContext'
import type { HttpClient } from './client'

export function useApi(): HttpClient {
  const client = useContext(ApiContext)
  if (!client) throw new Error('useApi must be used inside <ApiProvider>')
  return client
}
