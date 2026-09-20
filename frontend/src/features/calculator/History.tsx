import { useId } from 'react'
import styles from './History.module.css'
import { historyLabel, type HistoryEntry } from './model'

interface HistoryProps {
  /** Newest first. */
  entries: readonly HistoryEntry[]
  onActivate: (index: number) => void
}

/** The session's committed calculations; renders nothing while there are none. */
export function History({ entries, onActivate }: HistoryProps) {
  const headingId = useId()
  if (entries.length === 0) return null
  return (
    <section className={styles.history} aria-labelledby={headingId}>
      <h2 id={headingId} className={styles.heading}>
        History
      </h2>
      <ul className={styles.list}>
        {entries.map((entry, index) => (
          // Entries are immutable text buttons, so reusing a node by position is harmless.
          <li key={index}>
            <button
              type="button"
              className={styles.entry}
              onClick={() => {
                onActivate(index)
              }}
            >
              {historyLabel(entry)}
            </button>
          </li>
        ))}
      </ul>
    </section>
  )
}
