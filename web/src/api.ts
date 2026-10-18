// Typed client for the Go API. Shapes mirror the Go handlers' JSON.
import { serverText } from './i18n';
import { language } from './language';

export interface Me {
  email: string;
  /** False for accounts that only sign in with Google. */
  has_password: boolean;
  /** The server emailed a link to confirm the address, and it hasn't been opened yet. */
  confirm_email: boolean;
  /** A throwaway demo account full of sample data, deleted within a day. */
  demo?: boolean;
}

/** The ways this server offers to sign in. */
export interface AuthOptions {
  google: boolean;
  /** Visitors can try a demo without an account. */
  demo?: boolean;
  /** A forgotten password can be reset by email. */
  reset?: boolean;
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
    // The server words emails (and anything else it writes) in the language
    // the app shows.
    headers: {
      'Accept-Language': language(),
      ...(body === undefined ? {} : { 'Content-Type': 'application/json' }),
    },
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
  authOptions: () => request<AuthOptions>('GET', '/api/auth/options'),
  /** Signs in to a new demo account with sample data, in the app's language and this time zone. */
  startDemo: () => request<Me>('POST', '/api/demo', { tz: Intl.DateTimeFormat().resolvedOptions().timeZone }),
  resendConfirmation: () => request<void>('POST', '/api/verify-email'),
  /** Emails a link to choose a new password, if an account uses this address. */
  forgotPassword: (email: string) => request<void>('POST', '/api/password/forgot', { email }),
  /** Sets a new password with the emailed link's token, and signs in. */
  resetPassword: (token: string, password: string) => request<Me>('POST', '/api/password/reset', { token, password }),

  notes: () => request<Note[]>('GET', '/api/notes'),
  createNote: (body: string) => request<Note>('POST', '/api/notes', { body }),
  deleteNote: (id: number) => request<void>('DELETE', `/api/notes/${id}`),
};

/** A message to show for any thrown value, in the language in use. */
export function errorMessage(err: unknown): string {
  if (err instanceof ApiError) return serverText(err.message);
  return err instanceof Error ? err.message : String(err);
}
