const { test, expect, signUp } = require('./fixtures');

test('add and delete notes', async ({ page }) => {
  await signUp(page);
  await expect(page.getByText('No notes yet.')).toBeVisible();

  const box = page.getByLabel('New note');
  await box.fill('buy milk');
  await box.press('Enter');
  await box.fill('call mum');
  await page.getByRole('button', { name: 'Add' }).click();

  const notes = page.getByRole('list', { name: 'Notes' }).getByRole('listitem');
  await expect(notes).toHaveCount(2);
  await expect(notes.first()).toContainText('call mum'); // newest first

  await page.reload();
  await expect(notes).toHaveCount(2);

  await page.getByRole('button', { name: 'Delete note: buy milk' }).click();
  await expect(notes).toHaveCount(1);
  await expect(page.getByText('buy milk')).toHaveCount(0);
});
