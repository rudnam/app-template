import { useEffect, useState } from 'react';
import { api, ApiError, errorMessage, type Me } from './api';
import { AuthForm } from './components/AuthForm';
import { NotesPage } from './features/notes/NotesPage';

type Session =
  | { status: 'loading' }
  | { status: 'signed-out' }
  | { status: 'signed-in'; me: Me }
  | { status: 'error'; message: string };

export function App() {
  const [session, setSession] = useState<Session>({ status: 'loading' });

  useEffect(() => {
    api
      .me()
      .then((me) => setSession({ status: 'signed-in', me }))
      .catch((err) =>
        setSession(
          err instanceof ApiError && err.status === 401
            ? { status: 'signed-out' }
            : { status: 'error', message: errorMessage(err) },
        ),
      );
  }, []);

  async function signOut() {
    await api.signOut();
    setSession({ status: 'signed-out' });
  }

  return (
    <>
      <header className="topbar">
        <span className="brand">App Template</span>
        {session.status === 'signed-in' && (
          <span className="account">
            <span className="muted">{session.me.email}</span>
            <button className="link" onClick={signOut}>
              Sign out
            </button>
          </span>
        )}
      </header>
      <main className="page">
        {session.status === 'loading' && <p className="muted">Loading…</p>}
        {session.status === 'error' && (
          <p role="alert" className="error">
            Could not reach the server: {session.message}
          </p>
        )}
        {session.status === 'signed-out' && (
          <AuthForm onSignedIn={(me) => setSession({ status: 'signed-in', me })} />
        )}
        {session.status === 'signed-in' && <NotesPage />}
      </main>
    </>
  );
}
