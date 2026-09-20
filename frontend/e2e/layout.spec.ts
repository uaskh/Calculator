import { expect, test, type Locator, type Page } from '@playwright/test'
import { CalculatorPage } from './helpers.js'

/** Spec §7 "Layout" (decision 37): about 60 px keys on a laptop, never below 44 px. */
const LAPTOP_VIEWPORT = { width: 1280, height: 720 }
const PHONE_VIEWPORT = { width: 412, height: 915 }
const MIN_LAPTOP_KEY_HEIGHT_PX = 56

async function box(locator: Locator) {
  const rect = await locator.boundingBox()
  if (rect === null) throw new Error(`${locator.toString()} is not rendered`)
  return rect
}

/** The keypad's bounding box: the block that contains every key. */
function keypad(calculator: CalculatorPage): Locator {
  return calculator.key('equals').locator('..')
}

/** The calculator panel: the only child of the main landmark. */
function panel(page: Page): Locator {
  return page.getByRole('main').locator(':scope > *')
}

test.describe('desktop layout (decision 37)', () => {
  test('the calculator is one centred panel with the keypad left of History', async ({
    page,
  }, testInfo) => {
    await page.setViewportSize(LAPTOP_VIEWPORT)
    const calculator = new CalculatorPage(page, testInfo)
    await calculator.goto()
    await calculator.typeAndCommit('2+2')
    await expect(calculator.entry('2+2 = 4')).toBeVisible()

    await expect(panel(page)).toHaveCount(1)
    const rect = await box(panel(page))
    const viewportWidth = await page.evaluate(() => document.documentElement.clientWidth)
    const rightGap = viewportWidth - (rect.x + rect.width)
    expect(Math.abs(rect.x - rightGap)).toBeLessThanOrEqual(2)

    const keys = await box(keypad(calculator))
    const history = await box(calculator.history)
    expect(keys.x + keys.width).toBeLessThanOrEqual(history.x)
    // Their vertical ranges overlap: they sit side by side, not one above the other.
    expect(keys.y).toBeLessThan(history.y + history.height)
    expect(history.y).toBeLessThan(keys.y + keys.height)

    // The display spans the panel above both columns.
    const input = await box(calculator.input)
    expect(input.y + input.height).toBeLessThanOrEqual(keys.y)
    expect(input.y + input.height).toBeLessThanOrEqual(history.y)
    expect(input.width).toBeGreaterThan(keys.width)
  })

  test('the keypad does not move when the first history entry appears', async ({
    page,
  }, testInfo) => {
    await page.setViewportSize(LAPTOP_VIEWPORT)
    const calculator = new CalculatorPage(page, testInfo)
    await calculator.goto()
    const before = await box(keypad(calculator))

    await calculator.typeAndCommit('2+2')
    await expect(calculator.entry('2+2 = 4')).toBeVisible()

    expect(await box(keypad(calculator))).toEqual(before)
  })

  test(`keys are at least ${MIN_LAPTOP_KEY_HEIGHT_PX} px tall on a laptop`, async ({
    page,
  }, testInfo) => {
    await page.setViewportSize(LAPTOP_VIEWPORT)
    const calculator = new CalculatorPage(page, testInfo)
    await calculator.goto()

    for (const name of ['7', 'add', 'equals'] as const) {
      expect((await box(calculator.key(name))).height).toBeGreaterThanOrEqual(
        MIN_LAPTOP_KEY_HEIGHT_PX,
      )
    }
  })
})

test.describe('phone layout (decision 37)', () => {
  test('the keypad sits above History in a single column', async ({ page }, testInfo) => {
    await page.setViewportSize(PHONE_VIEWPORT)
    const calculator = new CalculatorPage(page, testInfo)
    await calculator.goto()
    await calculator.typeAndCommit('2+2')
    await expect(calculator.entry('2+2 = 4')).toBeVisible()

    const keys = await box(keypad(calculator))
    const history = await box(calculator.history)
    expect(keys.y + keys.height).toBeLessThanOrEqual(history.y)
    expect(Math.abs(keys.x - history.x)).toBeLessThanOrEqual(1)
  })
})
