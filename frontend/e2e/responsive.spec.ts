import { expect, test, type Page } from '@playwright/test'
import { CalculatorPage } from './helpers.js'

/** A valid 1,024-character expression: 511 × "1+" followed by "11". */
const LONG_EXPRESSION = `${'1+'.repeat(511)}11`
const LONG_EXPRESSION_RESULT = '522'

/** The longest result the contract allows: 118 characters (spec §7, UI-3). */
const MAXIMAL_INPUT = '-10^99-0.1234567890123456'
const MAXIMAL_RESULT = `-1${'0'.repeat(99)}.1234567890123456`

const VIEWPORTS = [
  { name: '320×640', size: { width: 320, height: 640 } },
  { name: '1280×720', size: { width: 1280, height: 720 } },
  { name: '1920×1080', size: { width: 1920, height: 1080 } },
  // The project's own viewport: Pixel 7 on mobile-chromium, Desktop Chrome on desktop-chromium.
  { name: 'the project viewport', size: null },
] as const

function pageOverflows(page: Page): Promise<boolean> {
  return page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth)
}

function scrollsInternally(page: Page, selector: 'input' | 'output'): Promise<boolean> {
  return page
    .locator(selector)
    .first()
    .evaluate((element) => element.scrollWidth > element.clientWidth)
}

test.beforeAll(() => {
  expect(LONG_EXPRESSION).toHaveLength(1024)
  expect(MAXIMAL_RESULT).toHaveLength(118)
})

for (const viewport of VIEWPORTS) {
  test.describe(`at ${viewport.name}`, () => {
    test('a 1,024-character expression scrolls inside the input, not the page', async ({
      page,
    }, testInfo) => {
      if (viewport.size) await page.setViewportSize(viewport.size)
      const calculator = new CalculatorPage(page, testInfo)
      await calculator.goto()

      await calculator.input.fill(LONG_EXPRESSION)
      await expect(calculator.input).toHaveValue(LONG_EXPRESSION)
      await expect(calculator.result).toHaveText(LONG_EXPRESSION_RESULT)

      expect(await scrollsInternally(page, 'input')).toBe(true)
      expect(await pageOverflows(page)).toBe(false)
    })

    test('a 118-character result is shown in full and scrolls inside the output', async ({
      page,
    }, testInfo) => {
      if (viewport.size) await page.setViewportSize(viewport.size)
      const calculator = new CalculatorPage(page, testInfo)
      await calculator.goto()

      await calculator.input.fill(MAXIMAL_INPUT)
      await expect(calculator.result).toHaveText(MAXIMAL_RESULT)

      expect(await scrollsInternally(page, 'output')).toBe(true)
      expect(await pageOverflows(page)).toBe(false)

      // The committed result and its history entry must not overflow the page either.
      await page.keyboard.press('Enter')
      await expect(calculator.input).toHaveValue(`(${MAXIMAL_RESULT})`)
      await expect(calculator.entry(`${MAXIMAL_INPUT} = ${MAXIMAL_RESULT}`)).toBeVisible()
      expect(await pageOverflows(page)).toBe(false)
    })
  })
}
