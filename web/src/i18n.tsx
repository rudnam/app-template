// Translating the app. Text is written in English, and the English is the
// key: t('Save') is "Save" in English and 保存 in Japanese. Every key must
// have an entry in locales/ja.ts, or the type check fails, so nothing is
// left untranslated by accident.
import { Fragment, useSyncExternalStore, type ReactNode } from 'react';
import { language, onLanguageChange, type Language } from './language';
import { ja, jaServer } from './locales/ja';

export type Key = keyof typeof ja;
type Vars = Record<string, string | number>;

/** The text in the language in use, with {name} filled in from vars. */
export function t(text: Key, vars?: Vars): string {
  const s: string = language() === 'ja' ? ja[text] : text;
  return vars ? s.replace(/\{(\w+)\}/g, (m, k: string) => (k in vars ? String(vars[k]) : m)) : s;
}

/** t for a count: one when n is 1, other otherwise, with {n} filled in. */
export function tn(n: number, one: Key, other: Key, vars?: Vars): string {
  return t(n === 1 ? one : other, { n, ...vars });
}

/** t with elements in it: tx('Sent to {email}.', { email: <strong>…</strong> }). */
export function tx(text: Key, vars: Record<string, ReactNode>): ReactNode {
  const s: string = language() === 'ja' ? ja[text] : text;
  return s.split(/(\{\w+\})/).map((part, i) => {
    const k = part.slice(1, -1);
    return <Fragment key={i}>{part.startsWith('{') && k in vars ? vars[k] : part}</Fragment>;
  });
}

/** The language in use; the component re-renders when it changes. */
export function useLanguage(): Language {
  return useSyncExternalStore(onLanguageChange, language);
}

// The server's messages are written in English (old app builds show them as
// they are). In Japanese, known ones are translated here; a pattern's {msg}
// is itself translated, e.g. the reason after "line 4: ".
const serverPatterns = Object.entries(jaServer)
  .filter(([en]) => en.includes('{'))
  .map(([en, translated]) => {
    const names: string[] = [];
    const source = en
      .split(/(\{\w+\})/)
      .map((part) => {
        if (!/^\{\w+\}$/.test(part)) return part.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
        names.push(part.slice(1, -1));
        return '(.+?)';
      })
      .join('');
    return { re: new RegExp(`^${source}$`), names, translated };
  });

/** A message from the server, in the language in use when it is a known one. */
export function serverText(message: string): string {
  if (language() !== 'ja') return message;
  const exact = (jaServer as Record<string, string>)[message];
  if (exact) return exact;
  for (const p of serverPatterns) {
    const m = message.match(p.re);
    if (!m) continue;
    return p.translated.replace(/\{(\w+)\}/g, (_, k: string) => {
      const v = m[p.names.indexOf(k) + 1] ?? '';
      return k === 'msg' ? serverText(v) : v;
    });
  }
  return message;
}
