import { config } from '../config'
import styles from './App.module.css'
import { ErrorBoundary } from './ErrorBoundary'

export function App() {
  return (
    <div className={styles.shell}>
      <header className={styles.header}>
        <h1 className={styles.title}>{config.appName}</h1>
      </header>
      <main className={styles.main}>
        <ErrorBoundary>{/* Feature components are composed here. */}</ErrorBoundary>
      </main>
    </div>
  )
}
