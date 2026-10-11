const { test, expect, uniqueEmail, PASSWORD } = require('./fixtures');

test('sign up, sign out and sign in again', async ({ page }) => {
  const email = uniqueEmail();
  await page.goto('/');
  await page.getByRole('button', { name: 'New here? Create an account' }).click();
  await page.getByLabel('Email').fill(email);
  await page.getByLabel('Password').fill(PASSWORD);
  await page.getByRole('button', { name: 'Create account' }).click();
  await expect(page.getByRole('heading', { name: 'Notes' })).toBeVisible();

  await page.getByRole('button', { name: 'Sign out' }).click();
  await expect(page.getByRole('heading', { name: 'Sign in' })).toBeVisible();

  await page.getByLabel('Email').fill(email);
  await page.getByLabel('Password').fill(PASSWORD);
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByText(email)).toBeVisible();

  await page.reload(); // the session survives a reload
  await expect(page.getByRole('heading', { name: 'Notes' })).toBeVisible();
});

test('a wrong password shows an error', async ({ page }) => {
  await page.goto('/');
  await page.getByLabel('Email').fill(uniqueEmail());
  await page.getByLabel('Password').fill('not the password');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('alert')).toHaveText('email or password is incorrect');
});
