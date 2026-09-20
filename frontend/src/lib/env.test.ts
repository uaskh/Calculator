import { describe, expect, it } from 'vitest'
import { apiBaseUrl, parsePositiveInt } from './env'

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

describe('apiBaseUrl', () => {
  it.each([
    { origin: undefined, expected: '/api/v1' },
    { origin: '', expected: '/api/v1' },
    { origin: '  ', expected: '/api/v1' },
    { origin: 'https://api.example.com', expected: 'https://api.example.com/api/v1' },
    { origin: 'https://api.example.com/', expected: 'https://api.example.com/api/v1' },
    { origin: 'http://localhost:8080//', expected: 'http://localhost:8080/api/v1' },
  ])('resolves $origin to $expected', ({ origin, expected }) => {
    expect(apiBaseUrl(origin)).toBe(expected)
  })
})
