import { describe, expect, it } from 'vitest'
import { parsePositiveInt } from './env'

describe('parsePositiveInt', () => {
  it.each([
    { raw: '5000', expected: 5000 },
    { raw: ' 42 ', expected: 42 },
  ])('parses $raw', ({ raw, expected }) => {
    expect(parsePositiveInt(raw, 1)).toBe(expected)
  })

  it.each([
    { raw: undefined },
    { raw: '' },
    { raw: '   ' },
    { raw: '0' },
    { raw: '-3' },
    { raw: '1.5' },
    { raw: 'abc' },
    { raw: '1e400' },
    { raw: '9007199254740993' },
  ])('falls back for $raw', ({ raw }) => {
    expect(parsePositiveInt(raw, 7)).toBe(7)
  })
})
