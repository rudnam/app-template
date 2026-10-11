// Typed client for the Go API. Shapes mirror the Go handlers' JSON.

export interface Me {
  email: string;
}

export interface Note {
  id: number;
  body: string;
  /** ISO 8601 timestamp. */
  created_at: string;
}

/** An error response from the API; message is safe to show the user. */
export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
  ) {
    super(message);
  }
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (res.status === 204) return undefined as T;
  const data = await res.json().catch(() => null);
  if (!res.ok) throw new ApiError(data?.error ?? res.statusText, res.status);
  return data as T;
}

export const api = {
  me: () => request<Me>('GET', '/api/me'),
  signUp: (email: string, password: string) => request<Me>('POST', '/api/signup', { email, password }),
  signIn: (email: string, password: string) => request<Me>('POST', '/api/login', { email, password }),
  signOut: () => request<void>('POST', '/api/logout'),

  notes: () => request<Note[]>('GET', '/api/notes'),
  createNote: (body: string) => request<Note>('POST', '/api/notes', { body }),
  deleteNote: (id: number) => request<void>('DELETE', `/api/notes/${id}`),
};

/** A message to show for any thrown value. */
export function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}
