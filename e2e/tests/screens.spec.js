// Screenshots of the main screens at phone and desktop width. They land in
// e2e/test-results/ (and in CI's artifacts), so you can look at a change
// without running anything. Add your own screens here.
const { test, expect, signUp } = require('./fixtures');

for (const [name, viewport] of [
  ['phone', { width: 390, height: 844 }],
  ['desktop', { width: 1280, height: 800 }],
]) {
  test(`screens at ${name} width`, async ({ page }) => {
    await page.setViewportSize(viewport);
    const shot = (label) => page.screenshot({ path: test.info().outputPath(`${name}-${label}.png`), fullPage: true });

    await page.goto('/');
    await expect(page.getByRole('heading', { name: 'Sign in' })).toBeVisible();
    await shot('1-sign-in');

    await signUp(page);
    await page.getByLabel('New note').fill('A note with a fairly long line of text, to check that it wraps');
    await page.getByRole('button', { name: 'Add' }).click();
    await expect(page.getByRole('listitem')).toHaveCount(1);
    await shot('2-notes');

    // Nothing may overflow sideways.
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
    expect(overflow).toBeLessThanOrEqual(0);
  });
}
