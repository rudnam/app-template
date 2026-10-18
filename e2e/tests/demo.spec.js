// The demo: anyone can try the app with sample data, in English or
// Japanese, without an account (DEMO, on by default when sign-up is).
const { test, expect } = require('./fixtures');

test('the introduction page opens a demo with sample data', async ({ page }) => {
  await page.goto('/');
  await page.getByRole('link', { name: 'Try the demo' }).first().click();
  await expect(page.getByRole('note')).toContainText('This is a demo with sample data.');
  await expect(page.getByText('Welcome to the demo!')).toBeVisible();
  await expect(page).toHaveURL(/\/app\/$/); // ?demo is gone, so a reload doesn't start another
  await expect(page.getByRole('banner')).toContainText('Demo');

  // It's a real account: notes can be added.
  await page.getByLabel('New note').fill('trying it out');
  await page.getByRole('button', { name: 'Add' }).click();
  await expect(page.getByText('trying it out')).toBeVisible();

  // From the demo to an account of one's own.
  await page.getByRole('button', { name: 'Create your account' }).click();
  await expect(page.getByRole('heading', { name: 'Create an account' })).toBeVisible();
});

test('the sign-in screen offers the demo, and ending it signs out', async ({ page }) => {
  await page.goto('/app/');
  await page.getByRole('button', { name: 'Try the demo' }).click();
  await expect(page.getByText('Welcome to the demo!')).toBeVisible();
  await page.getByRole('button', { name: 'End demo' }).click();
  await expect(page.getByRole('heading', { name: 'Sign in' })).toBeVisible();
  await page.reload();
  await expect(page.getByRole('heading', { name: 'Sign in' })).toBeVisible();
});

test.describe('in Japanese', () => {
  test.use({ locale: 'ja-JP' });

  test('the demo is in Japanese', async ({ page }) => {
    await page.goto('/app/');
    await page.getByRole('button', { name: 'デモを試す' }).click();
    await expect(page.getByRole('note')).toContainText('サンプルデータのデモです。');
    await expect(page.getByText('デモへようこそ！')).toBeVisible();
    await page.screenshot({ path: test.info().outputPath('demo-ja.png'), fullPage: true });
  });
});
