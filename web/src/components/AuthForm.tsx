import { useEffect, useState, type FormEvent } from 'react';
import { api, errorMessage, type Me } from '../api';
import { t } from '../i18n';
import type { Notice } from '../notice';

type Mode = 'sign-in' | 'sign-up' | 'forgot';

export function AuthForm({
  onSignedIn,
  onDemo,
  notice,
  mode: initialMode = 'sign-in',
}: {
  onSignedIn: (me: Me) => void;
  /** Starts a demo; offered when the server has demos. */
  onDemo?: () => void;
  notice?: Notice | null;
  mode?: Mode;
}) {
  const [mode, setMode] = useState<Mode>(initialMode);
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState(notice && !notice.ok ? notice.message : '');
  const [status, setStatus] = useState(notice?.ok ? notice.message : '');
  const [busy, setBusy] = useState(false);
  const [google, setGoogle] = useState(false);
  const [demo, setDemo] = useState(false);
  const [reset, setReset] = useState(false);

  useEffect(() => {
    api
      .authOptions()
      .then((o) => {
        setGoogle(o.google);
        setDemo(!!o.demo);
        setReset(!!o.reset);
      })
      .catch(() => {}); // the email form still works
  }, []);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError('');
    setStatus('');
    try {
      if (mode === 'forgot') {
        await api.forgotPassword(email);
        setStatus(t("If an account uses this email, we've sent it a link to choose a new password. The link works for an hour."));
        return;
      }
      onSignedIn(mode === 'sign-in' ? await api.signIn(email, password) : await api.signUp(email, password));
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  function switchTo(next: Mode) {
    setMode(next);
    setError('');
    setStatus('');
  }

  const signingIn = mode === 'sign-in';
  if (mode === 'forgot') {
    return (
      <form className="card stack" onSubmit={submit}>
        <h1>{t('Reset your password')}</h1>
        {status ? (
          <p role="status" className="notice">
            {status}
          </p>
        ) : (
          <p className="muted">{t("Enter your account's email, and we'll send you a link to choose a new password.")}</p>
        )}
        <label className="field">
          {t('Email')}
          <input type="email" autoComplete="email" required value={email} onChange={(e) => setEmail(e.target.value)} />
        </label>
        {error && (
          <p role="alert" className="error">
            {error}
          </p>
        )}
        <button type="submit" disabled={busy}>
          {status ? t('Send again') : t('Send link')}
        </button>
        <button type="button" className="link" onClick={() => switchTo('sign-in')}>
          {t('Back to sign in')}
        </button>
      </form>
    );
  }
  return (
    <form className="card stack" onSubmit={submit}>
      <h1>{signingIn ? t('Sign in') : t('Create an account')}</h1>
      {status && (
        <p role="status" className="notice">
          {status}
        </p>
      )}
      {google && (
        <>
          {/* A plain link: the server redirects to Google and back. */}
          <a className="button secondary google" href="/api/auth/google">
            <GoogleMark />
            {t('Continue with Google')}
          </a>
          <div className="or" aria-hidden="true">
            {t('or')}
          </div>
        </>
      )}
      <label className="field">
        {t('Email')}
        <input type="email" autoComplete="email" required value={email} onChange={(e) => setEmail(e.target.value)} />
      </label>
      <label className="field">
        {t('Password')}
        <input
          type="password"
          autoComplete={signingIn ? 'current-password' : 'new-password'}
          minLength={signingIn ? undefined : 10}
          required
          value={password}
          onChange={(e) => setPassword(e.target.value)}
        />
      </label>
      {error && (
        <p role="alert" className="error">
          {error}
        </p>
      )}
      <button type="submit" disabled={busy}>
        {signingIn ? t('Sign in') : t('Create account')}
      </button>
      {signingIn && reset && (
        <button type="button" className="link" onClick={() => switchTo('forgot')}>
          {t('Forgot your password?')}
        </button>
      )}
      <button type="button" className="link" onClick={() => switchTo(signingIn ? 'sign-up' : 'sign-in')}>
        {signingIn ? t('New here? Create an account') : t('Have an account? Sign in')}
      </button>
      {demo && onDemo && (
        <div className="demo-offer">
          <span className="muted small">{t('Just looking?')}</span>
          <button type="button" className="secondary" onClick={onDemo}>
            {t('Try the demo')}
          </button>
        </div>
      )}
    </form>
  );
}

/** Google's "G", in its own colours, as Google's branding rules ask. */
function GoogleMark() {
  return (
    <svg className="google-mark" viewBox="0 0 48 48" aria-hidden="true">
      <path fill="#EA4335" d="M24 9.5c3.54 0 6.71 1.22 9.21 3.6l6.85-6.85C35.9 2.38 30.47 0 24 0 14.62 0 6.51 5.38 2.56 13.22l7.98 6.19C12.43 13.72 17.74 9.5 24 9.5z" />
      <path fill="#4285F4" d="M46.98 24.55c0-1.57-.15-3.09-.38-4.55H24v9.02h12.94c-.58 2.96-2.26 5.48-4.78 7.18l7.73 6c4.51-4.18 7.09-10.36 7.09-17.65z" />
      <path fill="#FBBC05" d="M10.53 28.59c-.48-1.45-.76-2.99-.76-4.59s.27-3.14.76-4.59l-7.98-6.19C.92 16.46 0 20.12 0 24c0 3.88.92 7.54 2.56 10.78l7.97-6.19z" />
      <path fill="#34A853" d="M24 48c6.48 0 11.93-2.13 15.89-5.81l-7.73-6c-2.15 1.45-4.92 2.3-8.16 2.3-6.26 0-11.57-4.22-13.47-9.91l-7.98 6.19C6.51 42.62 14.62 48 24 48z" />
    </svg>
  );
}
