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
      // a 401 from /api/me means nobody is signed in, and a 400, 401, 403 or
      // 409 (e.g. a bad sign-in, a used reset link, or a refused change) is
      // shown in the form. Tests assert on what the user sees.
      if (m.type() === 'error' && !/status of 40[0139] /.test(m.text())) errors.push('console: ' + m.text());
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
  // Open the emailed link, so screens look as they do for a settled account.
  await page.request.get(await confirmationLink(page.request, email));
  await page.goto('/app/');
  await base.expect(page.getByRole('button', { name: 'Sign out' })).toBeVisible();
  return email;
}

const MAIL_API = process.env.MAIL_API || 'http://mail:8025'; // the mail catcher in e2e/compose.yml

/** The link in the newest email to "to" that has one matching pattern, read from the mail catcher. */
async function emailedLink(request, to, pattern) {
  let link = null;
  await base
    .expect(async () => {
      const found = await (await request.get(`${MAIL_API}/api/v1/search?query=${encodeURIComponent(`to:"${to}"`)}`)).json();
      for (const m of found.messages) {
        const text = (await (await request.get(`${MAIL_API}/api/v1/message/${m.ID}`)).json()).Text;
        link = text.match(pattern);
        if (link) break;
      }
      base.expect(link, `no link like ${pattern} emailed to ${to}`).not.toBeNull();
    })
    .toPass({ timeout: 10_000 });
  return link[0];
}

/** The newest confirmation link the app emailed to "to". */
function confirmationLink(request, to) {
  return emailedLink(request, to, /https?:\/\/\S+\/api\/verify-email\?token=[\w-]+/);
}

/** The newest password reset link the app emailed to "to". */
function resetLink(request, to) {
  return emailedLink(request, to, /https?:\/\/\S+\/app\/#\/reset\?token=[\w-]+/);
}

module.exports = { test, expect: base.expect, signUp, uniqueEmail, PASSWORD, confirmationLink, resetLink };
