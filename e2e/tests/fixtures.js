// Shared fixtures. Import { test, expect } from here instead of from
// @playwright/test so every test gets them.
const base = require('@playwright/test');

const test = base.test.extend({
  // Fails the test if the page throws or logs a console error (which includes
  // Content-Security-Policy violations).
  page: async ({ page }, use) => {
    const errors = [];
    page.on('pageerror', (e) => errors.push('pageerror: ' + e.message));
    page.on('console', (m) => {
      // The browser logs every 4xx response as an error, but some are expected:
      // a 401 from /api/me means nobody is signed in, and a 400 or 401 from a
      // bad sign-in is shown in the form. Tests assert on what the user sees.
      if (m.type() === 'error' && !/status of 40[01] /.test(m.text())) errors.push('console: ' + m.text());
    });
    await use(page);
    base.expect(errors, 'errors in the browser').toEqual([]);
  },
});

let n = 0;
/** A unique email per call, so parallel tests never share an account. */
function uniqueEmail() {
  return `user-${Date.now()}-${process.pid}-${n++}@example.com`;
}

const PASSWORD = 'a long password';

/**
 * Creates a fresh account through the API (the browser context gets its
 * session cookie) and opens the app signed in.
 */
async function signUp(page) {
  const email = uniqueEmail();
  const res = await page.request.post('/api/signup', { data: { email, password: PASSWORD } });
  base.expect(res.status(), await res.text()).toBe(201);
  await page.goto('/app/');
  await base.expect(page.getByRole('button', { name: 'Sign out' })).toBeVisible();
  return email;
}

module.exports = { test, expect: base.expect, signUp, uniqueEmail, PASSWORD };
