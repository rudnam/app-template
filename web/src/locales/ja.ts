// Japanese. Keys are the English the app is written in (see i18n.tsx); a
// key missing here fails the type check. {name} placeholders are filled in
// by t(); they may move, but keep their names.
export const ja = {
  // The shell and signing in
  'Loading…': '読み込み中…',
  'Could not reach the server: {error}': 'サーバーに接続できませんでした: {error}',
  'Sign out': 'ログアウト',
  Language: '言語',
  // The language picker's choice to follow the device's language.
  Auto: '自動',
  'Sign in': 'ログイン',
  'Create an account': 'アカウントを作成',
  Email: 'メールアドレス',
  Password: 'パスワード',
  'Create account': 'アカウントを作成',
  'New here? Create an account': 'はじめての方はこちら: アカウントを作成',
  'Have an account? Sign in': 'アカウントをお持ちの方: ログイン',

  // The example feature (notes): replace these with your own feature's
  Notes: 'メモ',
  'New note': '新しいメモ',
  'Write a note…': 'メモを書く…',
  Add: '追加',
  'No notes yet.': 'メモはまだありません。',
  'Delete note: {body}': 'メモを削除: {body}',
  Delete: '削除',
} as const;

// The server's messages, which are written in English (internal/, the
// httpx.Errorf calls). serverText() shows these in Japanese; {name} parts
// match anything, and {msg} is itself translated.
export const jaServer = {
  'sign in required': 'ログインしてください',
  'session expired; sign in again': 'セッションの有効期限が切れました。もう一度ログインしてください',
  'too many attempts; wait a minute and try again': '試行回数が多すぎます。1分ほど待ってからもう一度お試しください',
  'enter a valid email address': '有効なメールアドレスを入力してください',
  'sign-up is closed on this server': 'このサーバーでは新規登録を受け付けていません',
  'password must be {min} to {max} characters': 'パスワードは{min}〜{max}文字にしてください',
  'an account with this email already exists': 'このメールアドレスのアカウントはすでにあります',
  'email or password is incorrect': 'メールアドレスまたはパスワードが正しくありません',
  'a note must be 1 to {max} characters': 'メモは1〜{max}文字にしてください',
  'no such note': 'メモが見つかりません',
  'not found': '見つかりません',
} as const;
