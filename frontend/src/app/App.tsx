import { config } from '../config'
import { Calculator } from '../features/calculator/Calculator'
import styles from './App.module.css'
import { ErrorBoundary } from './ErrorBoundary'

export function App() {
  return (
    <div className={styles.shell}>
      <header className={styles.header}>
        <h1 className={styles.title}>{config.appName}</h1>
      </header>
      <main className={styles.main}>
        <ErrorBoundary>
          <Calculator />
        </ErrorBoundary>
      </main>
    </div>
  )
}
