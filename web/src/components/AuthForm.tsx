import { useState, type FormEvent } from 'react';
import { api, errorMessage, type Me } from '../api';

type Mode = 'sign-in' | 'sign-up';

export function AuthForm({ onSignedIn }: { onSignedIn: (me: Me) => void }) {
  const [mode, setMode] = useState<Mode>('sign-in');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      onSignedIn(mode === 'sign-in' ? await api.signIn(email, password) : await api.signUp(email, password));
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  const signingIn = mode === 'sign-in';
  return (
    <form className="card stack" onSubmit={submit}>
      <h1>{signingIn ? 'Sign in' : 'Create an account'}</h1>
      <label className="field">
        Email
        <input type="email" autoComplete="email" required value={email} onChange={(e) => setEmail(e.target.value)} />
      </label>
      <label className="field">
        Password
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
        {signingIn ? 'Sign in' : 'Create account'}
      </button>
      <button
        type="button"
        className="link"
        onClick={() => {
          setMode(signingIn ? 'sign-up' : 'sign-in');
          setError('');
        }}
      >
        {signingIn ? 'New here? Create an account' : 'Have an account? Sign in'}
      </button>
    </form>
  );
}
