const { test, expect } = require('./fixtures');

test('health check reports ok', async ({ request }) => {
  const res = await request.get('/healthz');
  expect(res.ok()).toBe(true);
  expect((await res.json()).status).toBe('ok');
});

test('the web app is installable: manifest and icons are served', async ({ request }) => {
  const manifest = await (await request.get('/manifest.webmanifest')).json();
  expect(manifest.display).toBe('standalone');
  for (const icon of manifest.icons) {
    expect((await request.get(icon.src)).ok(), icon.src).toBe(true);
  }
});

test('client-side routes load the app', async ({ page }) => {
  await page.goto('/some/deep/link');
  await expect(page.getByRole('heading', { name: 'Sign in' })).toBeVisible();
});
