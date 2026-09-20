import { describe, expect, it } from 'vitest'
import { asRecord, readFiniteNumber, readString } from './validate'

describe('response guards', () => {
  it('accepts well-formed values', () => {
    const record = asRecord({ name: 'a', value: 2 })
    expect(readString(record, 'name')).toBe('a')
    expect(readFiniteNumber(record, 'value')).toBe(2)
  })

  it.each([{ value: null }, { value: 'text' }, { value: 3 }, { value: [1] }])(
    'rejects $value as an object',
    ({ value }) => {
      expect(() => asRecord(value)).toThrow(TypeError)
    },
  )

  it('rejects missing or mistyped fields', () => {
    const record = asRecord({ name: 1, value: 'NaN', inf: Number.POSITIVE_INFINITY })
    expect(() => readString(record, 'name')).toThrow('name is not a string')
    expect(() => readFiniteNumber(record, 'value')).toThrow(TypeError)
    expect(() => readFiniteNumber(record, 'inf')).toThrow(TypeError)
    expect(() => readString(record, 'missing')).toThrow(TypeError)
  })
})
