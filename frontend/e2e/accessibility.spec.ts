import { AxeBuilder } from '@axe-core/playwright'
import { expect, test, type Page } from '@playwright/test'
import { CalculatorPage, DIVISION_BY_ZERO_MESSAGE } from './helpers.js'

const WCAG_TAGS = ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa']

/** Spec §8 NFR-1: stricter than the WCAG 2.2 AA minimum of 24 px. */
const MIN_TAP_TARGET_PX = 44

async function expectNoAxeViolations(page: Page): Promise<void> {
  const results = await new AxeBuilder({ page }).withTags(WCAG_TAGS).analyze()
  expect(results.violations).toEqual([])
}

async function reachErrorState(calculator: CalculatorPage): Promise<void> {
  await calculator.typeAndCommit('1/0')
  await expect(calculator.alert).toHaveText(DIVISION_BY_ZERO_MESSAGE)
}

test.describe('axe (WCAG 2.2 AA)', () => {
  test('initial state has no violations', async ({ page }, testInfo) => {
    const calculator = new CalculatorPage(page, testInfo)
    await calculator.goto()

    await expectNoAxeViolations(page)
  })

  test('live result has no violations', async ({ page }, testInfo) => {
    const calculator = new CalculatorPage(page, testInfo)
    await calculator.goto()
    await calculator.type('2*(3+4')
    await expect(calculator.result).toHaveText('14')
    await expect(page.getByText('Evaluated as 2*(3+4)')).toBeVisible()

    await expectNoAxeViolations(page)
  })

  test('commit error with the alert visible has no violations', async ({ page }, testInfo) => {
    const calculator = new CalculatorPage(page, testInfo)
    await calculator.goto()
    await reachErrorState(calculator)

    await expectNoAxeViolations(page)
  })

  test('history entries have no violations', async ({ page }, testInfo) => {
    const calculator = new CalculatorPage(page, testInfo)
    await calculator.goto()
    await calculator.typeAndCommit('2+2')
    await expect(calculator.entry('2+2 = 4')).toBeVisible()
    await page.keyboard.press('Escape')
    await calculator.typeAndCommit('2-7')
    await expect(calculator.history.getByRole('button')).toHaveText(['2-7 = -5', '2+2 = 4'])

    await expectNoAxeViolations(page)
  })

  test('dark scheme: initial state has no violations', async ({ page }, testInfo) => {
    await page.emulateMedia({ colorScheme: 'dark' })
    const calculator = new CalculatorPage(page, testInfo)
    await calculator.goto()

    await expectNoAxeViolations(page)
  })

  test('dark scheme: commit error has no violations', async ({ page }, testInfo) => {
    await page.emulateMedia({ colorScheme: 'dark' })
    const calculator = new CalculatorPage(page, testInfo)
    await calculator.goto()
    await reachErrorState(calculator)

    await expectNoAxeViolations(page)
  })

  test('reduced motion: initial state has no violations', async ({ page }, testInfo) => {
    await page.emulateMedia({ reducedMotion: 'reduce' })
    const calculator = new CalculatorPage(page, testInfo)
    await calculator.goto()

    await expectNoAxeViolations(page)
  })
})

test.describe('tap targets', () => {
  test(`every button is at least ${MIN_TAP_TARGET_PX} px in both dimensions`, async ({
    page,
  }, testInfo) => {
    const calculator = new CalculatorPage(page, testInfo)
    await calculator.goto()
    await calculator.typeAndCommit('2+2')
    await expect(calculator.entry('2+2 = 4')).toBeVisible()

    const buttons = page.getByRole('button')
    const count = await buttons.count()
    expect(count).toBe(24)

    const undersized: string[] = []
    for (let i = 0; i < count; i += 1) {
      const button = buttons.nth(i)
      const box = await button.boundingBox()
      const name = (await button.getAttribute('aria-label')) ?? (await button.innerText())
      if (box === null || box.width < MIN_TAP_TARGET_PX || box.height < MIN_TAP_TARGET_PX) {
        undersized.push(`${name}: ${box ? `${box.width}×${box.height}` : 'not rendered'}`)
      }
    }
    expect(undersized).toEqual([])
  })
})
