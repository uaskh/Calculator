/**
 * What is left behind for support when a request fails for a reason the UI cannot explain
 * to the user: enough to find the request in the backend logs, never the user's input.
 */
export interface FailureDetails {
  kind: string
  status: number | undefined
  code: string | undefined
  requestId: string | undefined
}

export type FailureReporter = (message: string, details: FailureDetails) => void

/** The default reporter: one structured console warning per failure. */
export const reportFailure: FailureReporter = (message, details) => {
  console.warn(message, details)
}
