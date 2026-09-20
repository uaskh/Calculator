import { describe, expect, it, vi } from 'vitest'
import { reportFailure } from './report'

describe('reportFailure', () => {
  it('writes one structured console warning', () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => undefined)
    const details = { kind: 'http', status: 503, code: 'TIMEOUT', requestId: 'abc123' }

    reportFailure('evaluate request failed', details)

    expect(warn).toHaveBeenCalledExactlyOnceWith('evaluate request failed', details)
  })
})
