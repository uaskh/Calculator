import { expect, test } from '@playwright/test'

test('renders the application shell without console errors', async ({ page }) => {
  const errors: string[] = []
  page.on('console', (message) => {
    if (message.type() === 'error') errors.push(message.text())
  })

  await page.goto('/')

  await expect(page.getByRole('banner')).toBeVisible()
  await expect(page.getByRole('main')).toBeVisible()
  expect(errors).toEqual([])
})

test('reaches the API through the web server proxy', async ({ request }) => {
  const response = await request.get('/api/v1/does-not-exist')

  expect(response.status()).toBe(404)
  expect(response.headers()['content-type']).toContain('application/problem+json')
  const body: unknown = await response.json()
  expect(body).toMatchObject({ status: 404, code: 'NOT_FOUND' })
})
