// The example feature's page. See internal/notes for the API side.
import { useEffect, useState, type FormEvent } from 'react';
import { api, errorMessage, type Note } from '../../api';
import { t } from '../../i18n';
import { locale } from '../../language';

export function NotesPage() {
  const [notes, setNotes] = useState<Note[] | null>(null);
  const [draft, setDraft] = useState('');
  const [error, setError] = useState('');

  useEffect(() => {
    api.notes().then(setNotes, (err) => setError(errorMessage(err)));
  }, []);

  async function add(e: FormEvent) {
    e.preventDefault();
    setError('');
    try {
      const note = await api.createNote(draft);
      setNotes((prev) => [note, ...(prev ?? [])]);
      setDraft('');
    } catch (err) {
      setError(errorMessage(err));
    }
  }

  async function remove(id: number) {
    setError('');
    try {
      await api.deleteNote(id);
      setNotes((prev) => prev?.filter((n) => n.id !== id) ?? null);
    } catch (err) {
      setError(errorMessage(err));
    }
  }

  return (
    <section className="stack">
      <h1>{t('Notes')}</h1>
      <form className="row" onSubmit={add}>
        <input
          aria-label={t('New note')}
          placeholder={t('Write a note…')}
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
        />
        <button type="submit" disabled={!draft.trim()}>
          {t('Add')}
        </button>
      </form>
      {error && (
        <p role="alert" className="error">
          {error}
        </p>
      )}
      {notes === null ? (
        <p className="muted">{t('Loading…')}</p>
      ) : notes.length === 0 ? (
        <p className="muted">{t('No notes yet.')}</p>
      ) : (
        <ul className="list" aria-label={t('Notes')}>
          {notes.map((n) => (
            <li key={n.id} className="card row">
              <span className="grow">{n.body}</span>
              <time className="muted" dateTime={n.created_at}>
                {new Date(n.created_at).toLocaleDateString(locale())}
              </time>
              <button className="link" aria-label={t('Delete note: {body}', { body: n.body })} onClick={() => remove(n.id)}>
                {t('Delete')}
              </button>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
