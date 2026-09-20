import { expect, type Locator, type Page, type TestInfo } from '@playwright/test'

/** The route every evaluation request goes through, for `page.route` failure simulation. */
export const EVALUATE_ROUTE = '**/api/v1/evaluate'

/** Message copy from spec §7 "Wording". */
export const UNAVAILABLE_MESSAGE = 'The calculator service is unavailable. Try again.'
export const DIVISION_BY_ZERO_MESSAGE = 'Cannot divide by zero.'

/** Keypad accessible names in spec §7 order (`=` last). */
export const KEYPAD_NAMES = [
  'clear',
  'backspace',
  'open parenthesis',
  'close parenthesis',
  '7',
  '8',
  '9',
  'divide',
  '4',
  '5',
  '6',
  'multiply',
  '1',
  '2',
  '3',
  'subtract',
  '0',
  'decimal point',
  'percent',
  'add',
  'sqrt, square root',
  'power',
  'equals',
] as const
export type KeypadName = (typeof KEYPAD_NAMES)[number]

/** Role and label locators for the single calculator screen. */
export class CalculatorPage {
  readonly input: Locator
  /** The `<output aria-label="Result">` live region. */
  readonly result: Locator
  /** The polite message shown while typing (a `<p role="status">`, distinct from the result). */
  readonly status: Locator
  readonly alert: Locator
  readonly history: Locator
  /** Every history entry button; empty while History shows its placeholder. */
  readonly entries: Locator
  /** The muted line History shows until the first commit (decision 41). */
  readonly historyPlaceholder: Locator

  readonly page: Page
  /** Touch is enabled by the device descriptor of the mobile project. */
  private readonly touch: boolean

  constructor(page: Page, testInfo: TestInfo) {
    this.page = page
    this.touch = Boolean(testInfo.project.use.hasTouch)
    this.input = page.getByRole('textbox', { name: 'Expression' })
    this.result = page.getByRole('status', { name: 'Result' })
    this.status = page.getByRole('status').and(page.locator('p'))
    this.alert = page.getByRole('alert')
    this.history = page.getByRole('region', { name: 'History' })
    this.entries = this.history.getByRole('button')
    this.historyPlaceholder = this.history.getByText('Your calculations will appear here')
  }

  async goto(): Promise<void> {
    await this.page.goto('/')
    await expect(this.input).toBeVisible()
  }

  key(name: KeypadName): Locator {
    return this.page.getByRole('button', { name, exact: true })
  }

  /** A history entry by its exact "expression = result" label. */
  entry(label: string): Locator {
    return this.history.getByRole('button', { name: label, exact: true })
  }

  /** Focuses the input and types with the keyboard, character by character. */
  async type(text: string): Promise<void> {
    await this.input.click()
    await this.page.keyboard.type(text)
  }

  /** Types `text` and commits it with Enter. */
  async typeAndCommit(text: string): Promise<void> {
    await this.type(text)
    await this.page.keyboard.press('Enter')
  }

  /** Presses a keypad button with a touch on touch projects and a mouse click elsewhere. */
  async tap(name: KeypadName): Promise<void> {
    const button = this.key(name)
    if (this.touch) {
      await button.tap()
      return
    }
    await button.click()
  }

  /** Whether the input has focus with its caret at the end of the text. */
  caretAtEnd(): Promise<boolean> {
    return this.input.evaluate(
      (element: HTMLInputElement) =>
        document.activeElement === element &&
        element.selectionStart === element.value.length &&
        element.selectionEnd === element.value.length,
    )
  }
}

/** True on the mobile project, where the device descriptor enables touch and a coarse pointer. */
export function isMobileProject(testInfo: TestInfo): boolean {
  return testInfo.project.name === 'mobile-chromium'
}

/** Presses Tab until `target` has focus, failing after `max` presses. */
export async function tabTo(page: Page, target: Locator, max = 40): Promise<void> {
  for (let i = 0; i < max; i += 1) {
    if (await target.evaluate((element) => element === document.activeElement)) return
    await page.keyboard.press('Tab')
  }
  throw new Error(`Tab did not reach ${target.toString()} within ${max} presses`)
}

/** The accessible name of the focused element (aria-label, then text), or its tag name. */
export function focusedName(page: Page): Promise<string> {
  return page.evaluate(() => {
    const active = document.activeElement
    if (!(active instanceof HTMLElement)) return ''
    return active.getAttribute('aria-label') ?? active.textContent?.trim() ?? active.tagName
  })
}
