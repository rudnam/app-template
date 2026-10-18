// Which language the app and the introduction page speak: the one the
// person picked (kept in this browser, so / and /app/ share it), or else
// the device's. English unless the device prefers Japanese.

export type Language = 'en' | 'ja';

/** Each language under its own name, as a picker lists it. */
export const languages: { code: Language; name: string }[] = [
  { code: 'en', name: 'English' },
  { code: 'ja', name: '日本語' },
];

const storageKey = 'app-template:language';

function saved(): Language | null {
  try {
    const v = localStorage.getItem(storageKey);
    return v === 'en' || v === 'ja' ? v : null;
  } catch {
    return null; // storage blocked: follow the device
  }
}

/** The device's language, if it is one the app speaks; English otherwise. */
export function deviceLanguage(): Language {
  for (const l of navigator.languages?.length ? navigator.languages : [navigator.language]) {
    if (/^ja\b/i.test(l)) return 'ja';
    if (/^en\b/i.test(l)) return 'en';
  }
  return 'en';
}

let current: Language = saved() ?? deviceLanguage();
const listeners = new Set<() => void>();
document.documentElement.lang = current;

/** The language in use now. */
export function language(): Language {
  return current;
}

/** The language the person picked, or null to follow the device. */
export function pickedLanguage(): Language | null {
  return saved();
}

/** Picks a language (null: follow the device) and redraws everything that uses it. */
export function setLanguage(l: Language | null) {
  try {
    if (l) localStorage.setItem(storageKey, l);
    else localStorage.removeItem(storageKey);
  } catch {
    // Not kept, but still used until the page is closed.
  }
  const next = l ?? deviceLanguage();
  if (next === current) return;
  current = next;
  document.documentElement.lang = next;
  for (const f of listeners) f();
}

/** Calls f whenever the language changes; returns a function that stops it. */
export function onLanguageChange(f: () => void): () => void {
  listeners.add(f);
  return () => listeners.delete(f);
}

/**
 * The locale dates and numbers are written in. In English it keeps the
 * device's English (en-GB writes 14 Oct), but not another language's.
 */
export function locale(): string {
  if (current === 'ja') return 'ja-JP';
  return (navigator.languages ?? [navigator.language]).find((l) => /^en\b/i.test(l)) ?? 'en';
}
