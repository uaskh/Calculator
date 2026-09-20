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

/** The dark palette applies on screen only, so print always gets the light one. */
const DARK_BLOCK = /@media screen and \(prefers-color-scheme: dark\)\s*{([\s\S]*?)}\s*}/

function themes(): { name: string; colours: Record<string, string> }[] {
  const dark = DARK_BLOCK.exec(tokens)
  if (!dark?.[1]) throw new Error('tokens.css has no screen-only dark block')
  const light = tokens.slice(0, dark.index)
  return [
    { name: 'light', colours: readBlock(light) },
    { name: 'dark', colours: readBlock(dark[1]) },
  ]
}

type Pair = [foreground: string, background: string]

const TEXT_PAIRS: Pair[] = [
  ['color-text', 'color-bg'],
  ['color-text', 'color-surface'],
  ['color-text', 'color-bg-gradient-start'],
  ['color-text', 'color-bg-gradient-end'],
  ['color-text', 'color-device-bg'],
  ['color-text', 'color-tape-bg'],
  ['color-text-muted', 'color-bg'],
  ['color-text-muted', 'color-surface'],
  ['color-text-muted', 'color-bg-gradient-start'],
  ['color-text-muted', 'color-bg-gradient-end'],
  ['color-text-muted', 'color-device-bg'],
  ['color-danger', 'color-bg'],
  ['color-danger', 'color-surface'],
  ['color-danger', 'color-device-bg'],
  ['color-success', 'color-bg'],
  ['color-accent-contrast', 'color-accent'],
  ['color-display-text', 'color-display-bg'],
  ['color-display-text-muted', 'color-display-bg'],
  ['color-key-text', 'color-key-bg'],
  ['color-key-operator-text', 'color-key-operator-bg'],
  ['color-key-action-text', 'color-key-action-bg'],
]

/** Borders, edges and state cues: non-text contrast (WCAG 1.4.11) against their background. */
const CONTROL_PAIRS: Pair[] = [
  ['color-control-border', 'color-bg'],
  ['color-key-edge', 'color-device-bg'],
  ['color-key-operator-edge', 'color-device-bg'],
  ['color-key-action-edge', 'color-device-bg'],
  ['color-accent-edge', 'color-device-bg'],
  ['color-display-border', 'color-display-bg'],
  ['color-display-danger', 'color-display-bg'],
  ['color-display-focus', 'color-display-bg'],
]

describe('design tokens (UI-6)', () => {
  it('force the light colour scheme when printing without repeating the palette', () => {
    const print = /@media print\s*{\s*:root\s*{([\s\S]*?)}\s*}/.exec(tokens)
    if (!print?.[1]) throw new Error('tokens.css has no print block')
    expect(print[1]).toMatch(/color-scheme:\s*light\s*;/)
    expect(readBlock(print[1])).toEqual({})
  })

  it('define every colour the pairs need in both themes', () => {
    for (const theme of themes()) {
      for (const pair of [...TEXT_PAIRS, ...CONTROL_PAIRS]) {
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

    it.each(CONTROL_PAIRS)('--%s on --%s reaches 3:1 (WCAG 1.4.11)', (control, background) => {
      const fg = colours[control]
      const bg = colours[background]
      if (fg === undefined || bg === undefined) throw new Error('token missing')
      expect(contrastRatio(fg, bg)).toBeGreaterThanOrEqual(3)
    })

    it('--color-display-text on --color-display-bg reaches 7:1 (LCD legibility)', () => {
      const fg = colours['color-display-text']
      const bg = colours['color-display-bg']
      if (fg === undefined || bg === undefined) throw new Error('token missing')
      expect(contrastRatio(fg, bg)).toBeGreaterThanOrEqual(7)
    })
  })
})
