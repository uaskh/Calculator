/** Colour maths for WCAG 2.x contrast checks over the design tokens. */

export interface Rgb {
  r: number
  g: number
  b: number
}

/** Parses `#rgb` or `#rrggbb` into 0–255 channels; throws on anything else. */
export function parseHex(hex: string): Rgb {
  const match = /^#([0-9a-f]{3}|[0-9a-f]{6})$/i.exec(hex.trim())
  if (!match?.[1]) throw new TypeError(`not a hex colour: ${hex}`)
  const digits = match[1].length === 3 ? [...match[1]].map((d) => d + d).join('') : match[1]
  return {
    r: Number.parseInt(digits.slice(0, 2), 16),
    g: Number.parseInt(digits.slice(2, 4), 16),
    b: Number.parseInt(digits.slice(4, 6), 16),
  }
}

function linear(channel: number): number {
  const c = channel / 255
  return c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4
}

/** WCAG relative luminance, 0 (black) to 1 (white). */
export function relativeLuminance({ r, g, b }: Rgb): number {
  return 0.2126 * linear(r) + 0.7152 * linear(g) + 0.0722 * linear(b)
}

/** WCAG contrast ratio between two hex colours, from 1 to 21. */
export function contrastRatio(foreground: string, background: string): number {
  const l1 = relativeLuminance(parseHex(foreground))
  const l2 = relativeLuminance(parseHex(background))
  const [lighter, darker] = l1 >= l2 ? [l1, l2] : [l2, l1]
  return (lighter + 0.05) / (darker + 0.05)
}
