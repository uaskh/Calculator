import { expect, test } from '@playwright/test'
import {
  CalculatorPage,
  DIVISION_BY_ZERO_MESSAGE,
  EVALUATE_ROUTE,
  UNAVAILABLE_MESSAGE,
} from './helpers.js'

const SERVICE_UNAVAILABLE = JSON.stringify({
  type: 'about:blank',
  title: 'Service Unavailable',
  status: 503,
  detail: 'The calculation took too long.',
  instance: '/api/v1/evaluate',
  code: 'TIMEOUT',
  requestId: '4f9c0e7d8a1b2c3d4e5f60718293a4b5',
})

/** Delay applied to every evaluation response, well past the 300 ms "Calculating…" rule. */
const SLOW_RESPONSE_MS = 1_500

test.describe('arithmetic and validation errors', () => {
  test('a failed commit is announced as an alert and keeps the input', async ({
    page,
  }, testInfo) => {
    const calculator = new CalculatorPage(page, testInfo)
    await calculator.goto()

    await calculator.type('1/0')
    await expect(calculator.status).toHaveText(DIVISION_BY_ZERO_MESSAGE)
    await expect(calculator.alert).toHaveCount(0)

    await page.keyboard.press('Enter')

    await expect(calculator.alert).toHaveText(DIVISION_BY_ZERO_MESSAGE)
    await expect(calculator.input).toHaveValue('1/0')
    await expect(calculator.input).toHaveAttribute('aria-invalid', 'true')
    await expect(calculator.input).toBeFocused()
    await expect(calculator.result).toHaveText('')
    await expect(calculator.entries).toHaveCount(0)
    const describedBy = await calculator.input.getAttribute('aria-describedby')
    expect(describedBy).not.toBeNull()
    await expect(calculator.alert).toHaveAttribute('id', describedBy ?? '')
  })

  test('validation feedback while typing is status text, not an alert', async ({
    page,
  }, testInfo) => {
    const calculator = new CalculatorPage(page, testInfo)
    await calculator.goto()

    await calculator.type('2+3)')

    await expect(calculator.status).toHaveText("unbalanced ')' at character 4")
    await expect(calculator.alert).toHaveCount(0)
    await expect(calculator.input).not.toHaveAttribute('aria-invalid', 'true')
    await expect(calculator.result).toHaveText('')
  })

  test('typing after a failed commit clears the alert', async ({ page }, testInfo) => {
    const calculator = new CalculatorPage(page, testInfo)
    await calculator.goto()
    await calculator.typeAndCommit('1/0')
    await expect(calculator.alert).toHaveText(DIVISION_BY_ZERO_MESSAGE)

    await page.keyboard.press('Backspace')
    await page.keyboard.type('2')

    await expect(calculator.alert).toHaveCount(0)
    await expect(calculator.input).not.toHaveAttribute('aria-invalid', 'true')
    await expect(calculator.input).toHaveValue('1/2')
    await expect(calculator.result).toHaveText('0.5')
  })

  test('an incomplete expression shows a blank result and no message', async ({
    page,
  }, testInfo) => {
    const calculator = new CalculatorPage(page, testInfo)
    await calculator.goto()

    await calculator.type('sqrt(')
    await page.keyboard.press('Enter')

    await expect(calculator.input).toHaveValue('sqrt(')
    await expect(calculator.result).toHaveText('')
    await expect(calculator.status).toHaveText('')
    await expect(calculator.alert).toHaveCount(0)
    await expect(calculator.entries).toHaveCount(0)
  })
})

test.describe('service failures', () => {
  test('a 5xx response is reported and the input survives', async ({ page }, testInfo) => {
    const calculator = new CalculatorPage(page, testInfo)
    await calculator.goto()
    await page.route(EVALUATE_ROUTE, (route) =>
      route.fulfill({
        status: 503,
        contentType: 'application/problem+json',
        body: SERVICE_UNAVAILABLE,
      }),
    )

    await calculator.type('2+2')
    await expect(calculator.status).toHaveText(UNAVAILABLE_MESSAGE)
    await expect(calculator.alert).toHaveCount(0)

    await page.keyboard.press('Enter')

    await expect(calculator.alert).toHaveText(UNAVAILABLE_MESSAGE)
    await expect(calculator.input).toHaveValue('2+2')
    await expect(calculator.input).not.toHaveAttribute('aria-invalid', 'true')
    await expect(calculator.input).toBeFocused()
    await expect(calculator.entries).toHaveCount(0)
  })

  test('a network failure is reported the same way', async ({ page }, testInfo) => {
    const calculator = new CalculatorPage(page, testInfo)
    await calculator.goto()
    await page.route(EVALUATE_ROUTE, (route) => route.abort('connectionrefused'))

    await calculator.typeAndCommit('2+2')

    await expect(calculator.alert).toHaveText(UNAVAILABLE_MESSAGE)
    await expect(calculator.input).toHaveValue('2+2')
    await expect(calculator.input).not.toHaveAttribute('aria-invalid', 'true')
  })

  test('recovers once the service answers again', async ({ page }, testInfo) => {
    const calculator = new CalculatorPage(page, testInfo)
    await calculator.goto()
    await page.route(EVALUATE_ROUTE, (route) => route.abort('connectionrefused'))
    await calculator.typeAndCommit('2+2')
    await expect(calculator.alert).toHaveText(UNAVAILABLE_MESSAGE)

    await page.unroute(EVALUATE_ROUTE)
    await page.keyboard.press('Enter')

    await expect(calculator.alert).toHaveCount(0)
    await expect(calculator.input).toHaveValue('4')
    await expect(calculator.entry('2+2 = 4')).toBeVisible()
  })

  test('a slow response shows "Calculating…" before the result', async ({ page }, testInfo) => {
    const calculator = new CalculatorPage(page, testInfo)
    await calculator.goto()
    await page.route(EVALUATE_ROUTE, async (route) => {
      const response = await route.fetch()
      await new Promise((resolve) => setTimeout(resolve, SLOW_RESPONSE_MS))
      await route.fulfill({ response })
    })

    await calculator.type('2+2')

    await expect(calculator.result).toHaveText('Calculating…')
    await expect(calculator.result).toHaveText('4', { timeout: SLOW_RESPONSE_MS * 4 })
  })

  test('the equals button is disabled while a commit is in flight', async ({ page }, testInfo) => {
    const calculator = new CalculatorPage(page, testInfo)
    await calculator.goto()
    await page.route(EVALUATE_ROUTE, async (route) => {
      const response = await route.fetch()
      await new Promise((resolve) => setTimeout(resolve, SLOW_RESPONSE_MS))
      await route.fulfill({ response })
    })

    await calculator.typeAndCommit('2+2')

    await expect(calculator.key('equals')).toBeDisabled()
    await expect(calculator.input).toHaveValue('4', { timeout: SLOW_RESPONSE_MS * 4 })
    await expect(calculator.key('equals')).toBeEnabled()
    await expect(calculator.entry('2+2 = 4')).toHaveCount(1)
  })
})
