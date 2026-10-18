const { test, expect, signUp, uniqueEmail, PASSWORD, confirmationLink, resetLink } = require('./fixtures');

test('sign up, sign out and sign in again', async ({ page }) => {
  const email = uniqueEmail();
  await page.goto('/app/');
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
  await expect(page.getByRole('heading', { name: 'Notes' })).toBeVisible();

  await page.reload(); // the session survives a reload
  await expect(page.getByRole('heading', { name: 'Notes' })).toBeVisible();
});

test('a wrong password shows an error', async ({ page }) => {
  await page.goto('/app/');
  await page.getByLabel('Email').fill(uniqueEmail());
  await page.getByLabel('Password').fill('not the password');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('alert')).toHaveText('email or password is incorrect');
});

test('a new account is asked to confirm its email, and the emailed link does it', async ({ page }) => {
  const email = uniqueEmail();
  await page.goto('/app/');
  await page.getByRole('button', { name: 'New here? Create an account' }).click();
  await page.getByLabel('Email').fill(email);
  await page.getByLabel('Password').fill(PASSWORD);
  await page.getByRole('button', { name: 'Create account' }).click();
  const reminder = page.getByText('Confirm your email: open the link we sent to');
  await expect(reminder).toBeVisible();
  await page.screenshot({ path: test.info().outputPath('confirm-email-reminder.png'), fullPage: true });

  const first = await confirmationLink(page.request, email);
  await page.getByRole('button', { name: 'Send again' }).click();
  await expect(page.getByText('Sent. Check your inbox at')).toBeVisible();
  await expect.poll(() => confirmationLink(page.request, email)).not.toBe(first);

  // Only the newest link works.
  await page.goto(first);
  await expect(page.getByRole('alert')).toHaveText('That confirmation link has expired or was already used.');
  await page.goto(await confirmationLink(page.request, email));
  await expect(page.getByRole('status')).toHaveText('Your email address is confirmed.');
  await expect(page).toHaveURL(/^[^?]*$/); // the notice is taken out of the address
  await expect(reminder).toBeHidden();
});

test('a forgotten password is reset with the emailed link, which signs out everywhere else', async ({ page, request }) => {
  const email = await signUp(page);
  // Also signed in somewhere else.
  expect((await request.post('/api/login', { data: { email, password: PASSWORD } })).status()).toBe(200);
  await page.request.post('/api/logout');

  await page.goto('/app/');
  await page.getByRole('button', { name: 'Forgot your password?' }).click();
  await expect(page.getByRole('heading', { name: 'Reset your password' })).toBeVisible();
  await page.getByLabel('Email').fill(email);
  await page.getByRole('button', { name: 'Send link' }).click();
  await expect(page.getByRole('status')).toContainText("If an account uses this email, we've sent it a link");

  const link = await resetLink(page.request, email);
  await page.goto(link);
  await expect(page.getByRole('heading', { name: 'Choose a new password' })).toBeVisible();
  await page.screenshot({ path: test.info().outputPath('reset-password.png'), fullPage: true });
  await page.getByLabel('New password').fill('a brand new password');
  await page.getByRole('button', { name: 'Save password' }).click();
  await expect(page.getByRole('status')).toHaveText('Your new password is saved.');
  await expect(page.getByRole('heading', { name: 'Notes' })).toBeVisible();
  await expect(page).not.toHaveURL(/token=/);
  expect((await request.get('/api/me')).status()).toBe(401);

  // The link works once.
  await page.goto(link);
  await page.getByLabel('New password').fill('yet another password');
  await page.getByRole('button', { name: 'Save password' }).click();
  await expect(page.getByRole('alert')).toHaveText('this link has expired or was already used; ask for a new one');
  await page.getByRole('button', { name: 'Back to sign in' }).click();
  await expect(page.getByRole('heading', { name: 'Notes' })).toBeVisible(); // still signed in with the new password
});

test('Continue with Google sends the browser to Google', async ({ page }) => {
  // Google itself is out of reach here; the Go tests play its part.
  await page.route('https://accounts.google.com/**', (route) => route.fulfill({ body: 'Google' }));
  await page.goto('/app/');
  const toGoogle = page.waitForRequest((r) => r.url().startsWith('https://accounts.google.com/'));
  await page.getByRole('link', { name: 'Continue with Google' }).click();
  const url = new URL((await toGoogle).url());
  expect(url.searchParams.get('scope')).toBe('openid email');
  expect(url.searchParams.get('client_id')).toBe('e2e-client.apps.googleusercontent.com');
  expect(url.searchParams.get('redirect_uri')).toBe('http://server:8080/api/auth/google/callback');
});

test('a failed Google sign-in says why on the sign-in page', async ({ page }) => {
  await page.goto('/api/auth/google/callback?error=access_denied');
  await expect(page.getByRole('alert')).toHaveText('Google sign-in was cancelled.');
  await expect(page).toHaveURL(/^[^?]*$/);
});
