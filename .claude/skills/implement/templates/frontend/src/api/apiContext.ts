import { createContext } from 'react'
import type { HttpClient } from './client'

export const ApiContext = createContext<HttpClient | null>(null)
