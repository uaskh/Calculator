/// <reference types="node" />
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'
import { contrastRatio, parseHex, relativeLuminance } from './contrast'

const tokens = readFileSync(resolve(import.meta.dirname, '../styles/tokens.css'), 'utf8')

describe('parseHex', () => {
  it.each([
    { hex: '#ffffff', expected: { r: 255, g: 255, b: 255 } },
    { hex: '#000', expected: { r: 0, g: 0, b: 0 } },
    { hex: ' #2554C7 ', expected: { r: 37, g: 84, b: 199 } },
  ])('parses $hex', ({ hex, expected }) => {
    expect(parseHex(hex)).toEqual(expected)
  })

  it.each(['', '#', '#12', '#12345', 'red', 'ffffff', '#gggggg'])('rejects %j', (hex) => {
    expect(() => parseHex(hex)).toThrow(TypeError)
  })
})

describe('relativeLuminance', () => {
  it('ranges from black to white', () => {
    expect(relativeLuminance(parseHex('#000000'))).toBe(0)
    expect(relativeLuminance(parseHex('#ffffff'))).toBeCloseTo(1, 10)
    expect(relativeLuminance(parseHex('#808080'))).toBeCloseTo(0.2158, 3)
  })
})

describe('contrastRatio', () => {
  it('matches the WCAG reference values', () => {
    expect(contrastRatio('#000000', '#ffffff')).toBeCloseTo(21, 5)
    expect(contrastRatio('#ffffff', '#000000')).toBeCloseTo(21, 5)
    expect(contrastRatio('#777777', '#ffffff')).toBeCloseTo(4.48, 2)
    expect(contrastRatio('#ffffff', '#ffffff')).toBe(1)
  })
})

/** Extracts `--name: #hex` pairs from a `:root { … }` block. */
function readBlock(css: string): Record<string, string> {
  const block: Record<string, string> = {}
  for (const match of css.matchAll(/--([\w-]+):\s*(#[0-9a-f]{3,6})\s*;/gi)) {
    if (match[1] !== undefined && match[2] !== undefined) block[match[1]] = match[2]
  }
  return block
}

function themes(): { name: string; colours: Record<string, string> }[] {
  const dark = /@media \(prefers-color-scheme: dark\)\s*{([\s\S]*?)}\s*}/.exec(tokens)
  if (!dark?.[1]) throw new Error('tokens.css has no dark block')
  const light = tokens.slice(0, dark.index)
  return [
    { name: 'light', colours: readBlock(light) },
    { name: 'dark', colours: readBlock(dark[1]) },
  ]
}

const TEXT_PAIRS: [foreground: string, background: string][] = [
  ['color-text', 'color-bg'],
  ['color-text', 'color-surface'],
  ['color-text-muted', 'color-bg'],
  ['color-text-muted', 'color-surface'],
  ['color-danger', 'color-bg'],
  ['color-danger', 'color-surface'],
  ['color-success', 'color-bg'],
  ['color-accent-contrast', 'color-accent'],
]

describe('design tokens (UI-6)', () => {
  it('define every colour the pairs need in both themes', () => {
    for (const theme of themes()) {
      for (const pair of TEXT_PAIRS) {
        for (const name of pair) {
          expect(theme.colours[name], `${theme.name} --${name}`).toMatch(/^#/)
        }
      }
    }
  })

  describe.each(themes())('$name theme', ({ colours }) => {
    it.each(TEXT_PAIRS)('--%s on --%s reaches 4.5:1', (foreground, background) => {
      const fg = colours[foreground]
      const bg = colours[background]
      if (fg === undefined || bg === undefined) throw new Error('token missing')
      expect(contrastRatio(fg, bg)).toBeGreaterThanOrEqual(4.5)
    })

    it('--color-control-border reaches 3:1 on --color-bg (WCAG 1.4.11)', () => {
      const border = colours['color-control-border']
      const bg = colours['color-bg']
      if (border === undefined || bg === undefined) throw new Error('token missing')
      expect(contrastRatio(border, bg)).toBeGreaterThanOrEqual(3)
    })
  })
})
