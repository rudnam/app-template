// Where the emailed reset link opens (#/reset?token=…): choose a new
// password, which signs in and signs out everywhere else.
import { useState, type FormEvent } from 'react';
import { api, errorMessage, type Me } from '../api';
import { t } from '../i18n';

export function ResetPasswordForm({ token, onDone, onCancel }: { token: string; onDone: (me: Me) => void; onCancel: () => void }) {
  const [password, setPassword] = useState('');
  const [error, setError] = useState(token ? '' : t('This link is incomplete. Open it from the email again, or ask for a new one.'));
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      onDone(await api.resetPassword(token, password));
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <form className="card stack" onSubmit={submit}>
      <h1>{t('Choose a new password')}</h1>
      <p className="muted">{t("You'll be signed out on your other devices.")}</p>
      <label className="field">
        {t('New password')}
        <input
          type="password"
          autoComplete="new-password"
          minLength={10}
          required
          autoFocus
          value={password}
          onChange={(e) => setPassword(e.target.value)}
        />
      </label>
      {error && (
        <p role="alert" className="error">
          {error}
        </p>
      )}
      <button type="submit" disabled={busy || !token}>
        {t('Save password')}
      </button>
      <button type="button" className="link" onClick={onCancel}>
        {t('Back to sign in')}
      </button>
    </form>
  );
}
