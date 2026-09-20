import { expect, test } from '@playwright/test'
import { CalculatorPage, isMobileProject } from './helpers.js'

test.describe('evaluating and committing expressions', () => {
  test('shows the live result while typing and continues from a committed result', async ({
    page,
  }, testInfo) => {
    const calculator = new CalculatorPage(page, testInfo)
    await calculator.goto()

    await calculator.type('2+3*4')
    await expect(calculator.result).toHaveText('14')

    await page.keyboard.press('Enter')
    await expect(calculator.input).toHaveValue('14')
    await expect(calculator.result).toHaveText('')
    await expect(calculator.entry('2+3*4 = 14')).toBeVisible()
    await expect(calculator.input).toBeFocused()

    await page.keyboard.type('*3')
    await expect(calculator.result).toHaveText('42')
    await page.keyboard.press('Enter')
    await expect(calculator.input).toHaveValue('42')
    await expect(calculator.entry('14*3 = 42')).toBeVisible()
  })

  test('the equals button commits like Enter and newest history comes first', async ({
    page,
  }, testInfo) => {
    const calculator = new CalculatorPage(page, testInfo)
    await calculator.goto()

    await calculator.type('2+2')
    await expect(calculator.result).toHaveText('4')
    await calculator.tap('equals')
    await expect(calculator.input).toHaveValue('4')
    await expect(calculator.entry('2+2 = 4')).toBeVisible()
    await expect(calculator.input).toBeFocused()

    await page.keyboard.type('*3')
    await calculator.tap('equals')
    await expect(calculator.input).toHaveValue('12')

    await expect(calculator.history.getByRole('button')).toHaveText(['4*3 = 12', '2+2 = 4'])
  })

  test('shows the normalized expression and records it in history', async ({ page }, testInfo) => {
    const calculator = new CalculatorPage(page, testInfo)
    await calculator.goto()

    await calculator.type('2*(3+4')
    await expect(calculator.result).toHaveText('14')
    await expect(page.getByText('Evaluated as 2*(3+4)')).toBeVisible()

    await page.keyboard.press('Enter')
    await expect(calculator.input).toHaveValue('14')
    await expect(page.getByText('Evaluated as 2*(3+4)')).toHaveCount(0)
    await expect(calculator.entry('2*(3+4) = 14')).toBeVisible()
  })

  test('wraps a negative committed result so it can be squared', async ({ page }, testInfo) => {
    const calculator = new CalculatorPage(page, testInfo)
    await calculator.goto()

    await calculator.typeAndCommit('2-7')
    await expect(calculator.input).toHaveValue('(-5)')
    await expect(calculator.entry('2-7 = -5')).toBeVisible()

    await page.keyboard.type('^2')
    await expect(calculator.result).toHaveText('25')
    await page.keyboard.press('Enter')
    await expect(calculator.input).toHaveValue('25')
    await expect(calculator.entry('(-5)^2 = 25')).toBeVisible()
  })

  test('Enter on an empty input does nothing', async ({ page }, testInfo) => {
    const calculator = new CalculatorPage(page, testInfo)
    await calculator.goto()

    await calculator.input.click()
    await page.keyboard.press('Enter')
    await page.keyboard.type('   ')
    await page.keyboard.press('Enter')

    await expect(calculator.input).toHaveValue('   ')
    await expect(calculator.entries).toHaveCount(0)
    await expect(calculator.alert).toHaveCount(0)
  })
})

test.describe('keypad', () => {
  test('builds an expression with taps and keeps focus on the input', async ({
    page,
  }, testInfo) => {
    const calculator = new CalculatorPage(page, testInfo)
    await calculator.goto()

    for (const name of [
      '7',
      'multiply',
      'open parenthesis',
      '2',
      'add',
      '1',
      'close parenthesis',
    ] as const) {
      await calculator.tap(name)
    }
    await expect(calculator.input).toHaveValue('7*(2+1)')
    await expect(calculator.result).toHaveText('21')
    await expect(calculator.input).toBeFocused()
    expect(await calculator.caretAtEnd()).toBe(true)

    await calculator.tap('clear')
    for (const name of ['sqrt, square root', '1', '6', 'close parenthesis'] as const) {
      await calculator.tap(name)
    }
    await expect(calculator.input).toHaveValue('sqrt(16)')
    await expect(calculator.result).toHaveText('4')
    await expect(calculator.input).toBeFocused()
  })

  test('hides the soft keyboard on touch devices only', async ({ page }, testInfo) => {
    const calculator = new CalculatorPage(page, testInfo)
    await calculator.goto()

    await expect(calculator.input).toHaveAttribute(
      'inputmode',
      isMobileProject(testInfo) ? 'none' : 'text',
    )
  })

  test('backspace removes one character', async ({ page }, testInfo) => {
    const calculator = new CalculatorPage(page, testInfo)
    await calculator.goto()

    await calculator.type('12+3')
    await calculator.tap('backspace')
    await expect(calculator.input).toHaveValue('12+')

    await calculator.tap('clear')
    await calculator.tap('sqrt, square root')
    await expect(calculator.input).toHaveValue('sqrt(')
    await calculator.tap('backspace')
    await expect(calculator.input).toHaveValue('sqrt')
  })
})

test.describe('clearing', () => {
  test('C empties the input, result and message but keeps history', async ({ page }, testInfo) => {
    const calculator = new CalculatorPage(page, testInfo)
    await calculator.goto()
    await calculator.typeAndCommit('2+2')
    await expect(calculator.entry('2+2 = 4')).toBeVisible()

    await page.keyboard.type('+3)')
    await expect(calculator.input).toHaveValue('4+3)')
    await expect(calculator.status).toHaveText("unbalanced ')' at character 4")

    await calculator.tap('clear')
    await expect(calculator.input).toHaveValue('')
    await expect(calculator.result).toHaveText('')
    await expect(calculator.status).toHaveText('')
    await expect(calculator.alert).toHaveCount(0)
    await expect(calculator.input).toBeFocused()
    await expect(calculator.entry('2+2 = 4')).toBeVisible()
  })

  test('Escape clears the way C does', async ({ page }, testInfo) => {
    const calculator = new CalculatorPage(page, testInfo)
    await calculator.goto()
    await calculator.typeAndCommit('1/0')
    await expect(calculator.alert).toHaveText('Cannot divide by zero.')

    await page.keyboard.press('Escape')
    await expect(calculator.input).toHaveValue('')
    await expect(calculator.result).toHaveText('')
    await expect(calculator.alert).toHaveCount(0)
    await expect(calculator.input).not.toHaveAttribute('aria-invalid', 'true')
    await expect(calculator.input).toBeFocused()
  })
})

test.describe('history', () => {
  test('activating an entry reloads its expression with the caret at the end', async ({
    page,
  }, testInfo) => {
    const calculator = new CalculatorPage(page, testInfo)
    await calculator.goto()
    await calculator.typeAndCommit('2+2')
    await expect(calculator.entry('2+2 = 4')).toBeVisible()
    await page.keyboard.press('Escape')
    await calculator.typeAndCommit('3*3')
    await expect(calculator.history.getByRole('button')).toHaveText(['3*3 = 9', '2+2 = 4'])

    await calculator.entry('2+2 = 4').click()

    await expect(calculator.input).toHaveValue('2+2')
    await expect(calculator.input).toBeFocused()
    expect(await calculator.caretAtEnd()).toBe(true)
    await expect(calculator.result).toHaveText('4')
  })

  test('keeps duplicate commits and is emptied by a reload', async ({ page }, testInfo) => {
    const calculator = new CalculatorPage(page, testInfo)
    await calculator.goto()
    await calculator.typeAndCommit('2+2')
    await expect(calculator.entry('2+2 = 4')).toHaveCount(1)
    await page.keyboard.press('Escape')
    await calculator.typeAndCommit('2+2')
    await expect(calculator.entry('2+2 = 4')).toHaveCount(2)

    await page.reload()

    await expect(calculator.input).toBeVisible()
    await expect(calculator.entries).toHaveCount(0)
  })
})
