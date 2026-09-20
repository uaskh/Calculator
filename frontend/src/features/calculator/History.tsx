import { useId } from 'react'
import styles from './History.module.css'
import { historyLabel, type HistoryEntry } from './model'

interface HistoryProps {
  /** Newest first. */
  entries: readonly HistoryEntry[]
  onActivate: (index: number) => void
}

/**
 * The session's committed calculations. While there are none the section keeps its heading
 * and shows a muted placeholder line, so the column never looks empty (decision 41).
 */
export function History({ entries, onActivate }: HistoryProps) {
  const headingId = useId()
  return (
    <section className={styles.history} aria-labelledby={headingId}>
      <h2 id={headingId} className={styles.heading}>
        History
      </h2>
      {entries.length === 0 ? (
        <p className={styles.placeholder}>Your calculations will appear here</p>
      ) : (
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
      )}
    </section>
  )
}
