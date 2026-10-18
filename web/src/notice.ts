// Messages the server sends back in the address after a redirect: from
// Google sign-in (?signin_error=…) and from the link in a confirmation email
// (?email=…). Read once when the app loads, then removed from the address so
// a reload doesn't show them again.
import { t, type Key } from './i18n';

export interface Notice {
  message: string;
  ok: boolean;
}

/** What went wrong with Google sign-in, by the server's reason code. */
export const signinErrors: Record<string, Key> = {
  cancelled: 'Google sign-in was cancelled.',
  expired: 'That sign-in attempt expired. Please try again.',
  closed: "This server isn't taking new accounts.",
  unverified: "Your Google account's email address isn't verified with Google.",
  linked: 'An account with this email is already linked to a different Google account.',
  busy: 'Too many attempts. Wait a minute and try again.',
  failed: "Couldn't sign in with Google. Please try again.",
};

const emailResults: Record<string, { message: Key; ok: boolean }> = {
  confirmed: { message: 'Your email address is confirmed.', ok: true },
  expired: { message: 'That confirmation link has expired or was already used.', ok: false },
  failed: { message: "Couldn't confirm your email. Please try again.", ok: false },
};

function take(): Notice | null {
  const params = new URLSearchParams(location.search);
  const signin = params.get('signin_error');
  const email = params.get('email');
  if (signin === null && email === null) return null;
  history.replaceState(null, '', location.pathname + location.hash);
  if (signin !== null) return { message: t(signinErrors[signin] ?? signinErrors.failed), ok: false };
  const result = emailResults[email ?? ''] ?? emailResults.failed;
  return { message: t(result.message), ok: result.ok };
}

/** The notice the app was opened with, if any. */
export const initialNotice = take();
