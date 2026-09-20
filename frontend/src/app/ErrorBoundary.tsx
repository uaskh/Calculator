import { Component, type ErrorInfo, type ReactNode } from 'react'
import styles from './ErrorBoundary.module.css'

interface ErrorBoundaryProps {
  children?: ReactNode
}

interface ErrorBoundaryState {
  failed: boolean
}

/** Catches rendering errors below it and offers a way to recover. */
export class ErrorBoundary extends Component<ErrorBoundaryProps, ErrorBoundaryState> {
  override state: ErrorBoundaryState = { failed: false }

  static getDerivedStateFromError(): ErrorBoundaryState {
    return { failed: true }
  }

  override componentDidCatch(error: Error, info: ErrorInfo): void {
    if (import.meta.env.DEV) console.error(error, info.componentStack)
  }

  private readonly retry = (): void => {
    this.setState({ failed: false })
  }

  override render(): ReactNode {
    if (!this.state.failed) return this.props.children
    return (
      <div role="alert" className={styles.fallback}>
        <p>Something went wrong while showing this part of the page.</p>
        <button type="button" className={styles.retry} onClick={this.retry}>
          Try again
        </button>
      </div>
    )
  }
}
