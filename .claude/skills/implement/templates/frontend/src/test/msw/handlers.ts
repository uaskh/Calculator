import type { RequestHandler } from 'msw'

/**
 * Default happy-path handlers shared by all tests, one per API operation. Tests override
 * them with server.use(...) to simulate errors, slow responses or unusual payloads.
 */
export const handlers: RequestHandler[] = []
