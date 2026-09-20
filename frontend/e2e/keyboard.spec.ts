import { expect, test } from '@playwright/test'
import { CalculatorPage, focusedName, KEYPAD_NAMES, tabTo } from './helpers.js'

test.describe('keyboard-only use', () => {
  test('types, commits with Enter and clears with Escape without a pointer', async ({
    page,
  }, testInfo) => {
    const calculator = new CalculatorPage(page, testInfo)
    await calculator.goto()

    await tabTo(page, calculator.input)
    await page.keyboard.type('2+3*4')
    await expect(calculator.result).toHaveText('14')

    await page.keyboard.press('Enter')
    await expect(calculator.input).toHaveValue('14')
    await expect(calculator.entry('2+3*4 = 14')).toBeVisible()
    await expect(calculator.input).toBeFocused()

    await page.keyboard.press('Escape')
    await expect(calculator.input).toHaveValue('')
    await expect(calculator.result).toHaveText('')
    await expect(calculator.input).toBeFocused()
  })

  test('Tab moves from the input to the result and then through the keypad in order', async ({
    page,
  }, testInfo) => {
    const calculator = new CalculatorPage(page, testInfo)
    await calculator.goto()
    await tabTo(page, calculator.input)

    await page.keyboard.press('Tab')
    await expect(calculator.result).toBeFocused()

    for (const name of KEYPAD_NAMES.slice(0, 4)) {
      await page.keyboard.press('Tab')
      await expect(calculator.key(name)).toBeFocused()
    }
  })

  test('every keypad button is reachable with Tab', async ({ page }, testInfo) => {
    const calculator = new CalculatorPage(page, testInfo)
    await calculator.goto()
    await tabTo(page, calculator.input)

    const focused: string[] = []
    // Input → result → 23 keys; a few extra presses prove the order ends where expected.
    for (let i = 0; i < KEYPAD_NAMES.length + 3; i += 1) {
      await page.keyboard.press('Tab')
      focused.push(await focusedName(page))
    }

    const keys = focused.filter((name) => (KEYPAD_NAMES as readonly string[]).includes(name))
    expect(keys).toEqual([...KEYPAD_NAMES])
  })

  test('Enter on the equals button commits', async ({ page }, testInfo) => {
    const calculator = new CalculatorPage(page, testInfo)
    await calculator.goto()
    await tabTo(page, calculator.input)
    await page.keyboard.type('6*7')
    await expect(calculator.result).toHaveText('42')

    await tabTo(page, calculator.key('equals'))
    await page.keyboard.press('Enter')

    await expect(calculator.input).toHaveValue('42')
    await expect(calculator.entry('6*7 = 42')).toBeVisible()
    await expect(calculator.input).toBeFocused()
  })

  test('Space and Enter activate a history entry', async ({ page }, testInfo) => {
    const calculator = new CalculatorPage(page, testInfo)
    await calculator.goto()
    await tabTo(page, calculator.input)
    await page.keyboard.type('2+2')
    await page.keyboard.press('Enter')
    await expect(calculator.input).toHaveValue('4')
    await page.keyboard.press('Escape')
    await page.keyboard.type('3*3')
    await page.keyboard.press('Enter')
    await expect(calculator.input).toHaveValue('9')
    await page.keyboard.press('Escape')
    await expect(calculator.history.getByRole('button')).toHaveText(['3*3 = 9', '2+2 = 4'])

    await tabTo(page, calculator.entry('2+2 = 4'))
    await page.keyboard.press('Enter')
    await expect(calculator.input).toHaveValue('2+2')
    await expect(calculator.input).toBeFocused()
    await expect(calculator.result).toHaveText('4')

    await tabTo(page, calculator.entry('3*3 = 9'))
    await page.keyboard.press('Space')
    await expect(calculator.input).toHaveValue('3*3')
    await expect(calculator.input).toBeFocused()
    expect(await calculator.caretAtEnd()).toBe(true)
    await expect(calculator.result).toHaveText('9')
  })

  test('Shift+Tab from the first key returns to the result and the input', async ({
    page,
  }, testInfo) => {
    const calculator = new CalculatorPage(page, testInfo)
    await calculator.goto()
    await tabTo(page, calculator.key('clear'))

    await page.keyboard.press('Shift+Tab')
    await expect(calculator.result).toBeFocused()
    await page.keyboard.press('Shift+Tab')
    await expect(calculator.input).toBeFocused()
  })
})
