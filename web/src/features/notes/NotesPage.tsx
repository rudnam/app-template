// The example feature's page. See internal/notes for the API side.
import { useEffect, useState, type FormEvent } from 'react';
import { api, errorMessage, type Note } from '../../api';

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
      <h1>Notes</h1>
      <form className="row" onSubmit={add}>
        <input
          aria-label="New note"
          placeholder="Write a note…"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
        />
        <button type="submit" disabled={!draft.trim()}>
          Add
        </button>
      </form>
      {error && (
        <p role="alert" className="error">
          {error}
        </p>
      )}
      {notes === null ? (
        <p className="muted">Loading…</p>
      ) : notes.length === 0 ? (
        <p className="muted">No notes yet.</p>
      ) : (
        <ul className="list" aria-label="Notes">
          {notes.map((n) => (
            <li key={n.id} className="card row">
              <span className="grow">{n.body}</span>
              <time className="muted" dateTime={n.created_at}>
                {new Date(n.created_at).toLocaleDateString()}
              </time>
              <button className="link" aria-label={`Delete note: ${n.body}`} onClick={() => remove(n.id)}>
                Delete
              </button>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
