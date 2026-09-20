import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { ApiProvider } from './api/ApiProvider'
import { createHttpClient } from './api/client'
import { App } from './app/App'
import { config } from './config'
import './styles/tokens.css'
import './styles/global.css'

const container = document.getElementById('root')
if (!container) throw new Error('index.html must contain an element with id "root"')

const client = createHttpClient({
  baseUrl: config.apiBaseUrl,
  timeoutMs: config.requestTimeoutMs,
})

createRoot(container).render(
  <StrictMode>
    <ApiProvider client={client}>
      <App />
    </ApiProvider>
  </StrictMode>,
)
