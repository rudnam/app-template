import { useEffect, useState } from 'react';
import { api, ApiError, errorMessage, type Me } from './api';
import { AuthForm } from './components/AuthForm';
import { LanguagePicker } from './components/LanguagePicker';
import { ResetPasswordForm } from './components/ResetPasswordForm';
import { NotesPage } from './features/notes/NotesPage';
import { t, tx, useLanguage } from './i18n';
import { initialNotice, type Notice } from './notice';
import { replaceRoute, useRoute } from './router';

/**
 * The introduction page's "Try the demo" opens /app/?demo. Read once when
 * the app loads, then dropped from the address so a reload doesn't start
 * another demo.
 */
const demoRequested = (() => {
  const params = new URLSearchParams(location.search);
  if (!params.has('demo')) return false;
  params.delete('demo');
  const qs = params.toString();
  history.replaceState(null, '', location.pathname + (qs ? `?${qs}` : '') + location.hash);
  return true;
})();

type Session =
  | { status: 'loading'; demo?: boolean }
  | { status: 'signed-out'; notice?: Notice | null; mode?: 'sign-up' }
  | { status: 'signed-in'; me: Me; notice?: Notice | null }
  | { status: 'error'; message: string };

export function App() {
  const [session, setSession] = useState<Session>({ status: 'loading' });
  const route = useRoute();
  useLanguage(); // redraw everything when the language changes

  useEffect(() => {
    api
      .me()
      // e.g. "Your email address is confirmed", after opening the emailed link.
      .then((me) => setSession({ status: 'signed-in', me, notice: initialNotice }))
      .catch((err) => {
        if (!(err instanceof ApiError && err.status === 401)) {
          setSession({ status: 'error', message: errorMessage(err) });
          return;
        }
        if (demoRequested) {
          startDemo();
          return;
        }
        setSession({ status: 'signed-out', notice: initialNotice });
      });
  }, []);

  /** Signs in to a new demo account full of sample data. */
  async function startDemo() {
    setSession({ status: 'loading', demo: true });
    try {
      setSession({ status: 'signed-in', me: await api.startDemo() });
    } catch (err) {
      setSession({ status: 'signed-out', notice: { message: errorMessage(err), ok: false } });
    }
  }

  /** From a demo to the sign-up form. Signing out deletes the demo. */
  async function leaveDemo() {
    await api.signOut();
    setSession({ status: 'signed-out', mode: 'sign-up' });
  }

  async function signOut() {
    await api.signOut();
    setSession({ status: 'signed-out' });
  }

  // The emailed reset link, whether or not this browser is signed in.
  const resetting = route.page === 'reset' && (session.status === 'signed-in' || session.status === 'signed-out');

  return (
    <>
      <header className="topbar">
        <a className="brand" href="/">
          App Template
        </a>
        <span className="account">
          {session.status === 'signed-in' && (
            <>
              <span className="muted">{session.me.demo ? t('Demo') : session.me.email}</span>
              <button className="link" onClick={signOut}>
                {session.me.demo ? t('End demo') : t('Sign out')}
              </button>
            </>
          )}
          <LanguagePicker />
        </span>
      </header>
      <main className="page">
        {session.status === 'loading' && (
          <p className="muted">{session.demo ? t('Setting up your demo…') : t('Loading…')}</p>
        )}
        {session.status === 'error' && (
          <p role="alert" className="error">
            {t('Could not reach the server: {error}', { error: session.message })}
          </p>
        )}
        {resetting && (
          <ResetPasswordForm
            token={route.params.get('token') ?? ''}
            onDone={(me) => {
              // Off the reset page, without leaving the used link in the history.
              replaceRoute('home');
              setSession({ status: 'signed-in', me, notice: { message: t('Your new password is saved.'), ok: true } });
            }}
            onCancel={() => replaceRoute('home')}
          />
        )}
        {!resetting && session.status === 'signed-out' && (
          <AuthForm
            key={`${session.mode}-${session.notice?.message}`}
            mode={session.mode}
            notice={session.notice}
            onSignedIn={(me) => setSession({ status: 'signed-in', me })}
            onDemo={startDemo}
          />
        )}
        {!resetting && session.status === 'signed-in' && (
          <div className="stack">
            {session.notice && (
              <p role={session.notice.ok ? 'status' : 'alert'} className={session.notice.ok ? 'notice' : 'notice error'}>
                {session.notice.message}
              </p>
            )}
            {session.me.demo && <DemoBanner onCreateAccount={leaveDemo} />}
            {session.me.confirm_email && <ConfirmEmail email={session.me.email} />}
            <NotesPage />
          </div>
        )}
      </main>
    </>
  );
}

/** In a demo: what this is, and the way to an account of one's own. */
function DemoBanner({ onCreateAccount }: { onCreateAccount: () => void }) {
  return (
    <div className="notice row" role="note">
      <span className="grow">
        <strong>{t('This is a demo with sample data.')}</strong>{' '}
        {t('Change anything you like: nobody else sees it, and it’s deleted within a day.')}
      </span>
      <button type="button" className="secondary" onClick={onCreateAccount}>
        {t('Create your account')}
      </button>
    </div>
  );
}

/** A reminder to open the link we emailed, with a way to send it again. */
function ConfirmEmail({ email }: { email: string }) {
  const [state, setState] = useState<'idle' | 'sending' | 'sent' | string>('idle');
  async function resend() {
    setState('sending');
    try {
      await api.resendConfirmation();
      setState('sent');
    } catch (err) {
      setState(errorMessage(err));
    }
  }
  return (
    <div className="notice row">
      <span className="grow">
        {state === 'sent'
          ? tx('Sent. Check your inbox at {email}.', { email: <strong>{email}</strong> })
          : tx('Confirm your email: open the link we sent to {email}.', { email: <strong>{email}</strong> })}
        {state !== 'idle' && state !== 'sending' && state !== 'sent' && <span className="error block">{state}</span>}
      </span>
      {state !== 'sent' && (
        <button type="button" className="link" disabled={state === 'sending'} onClick={resend}>
          {t('Send again')}
        </button>
      )}
    </div>
  );
}
