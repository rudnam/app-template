// The introduction page at /. Links meant for the app (/#/...) and the app
// installed to a home screen are sent on to /app/.
import './styles.css';
import './landing.css';
import { language, setLanguage } from './language';

const forApp = location.hash.startsWith('#/') || matchMedia('(display-mode: standalone)').matches;
if (forApp) location.replace('/app/' + location.search + location.hash);
else {
  translate();
  document.querySelector('#language-toggle')?.addEventListener('click', () => {
    setLanguage(language() === 'ja' ? 'en' : 'ja');
    translate();
  });
  revealOnScroll();
  offerDemo();
}

// "Try the demo" shows only when the server offers demos (DEMO).
function offerDemo() {
  fetch('/api/auth/options')
    .then((r) => (r.ok ? r.json() : null))
    .then((o: { demo?: boolean } | null) => {
      if (o?.demo) document.querySelectorAll<HTMLElement>('[data-demo-hidden]').forEach((el) => (el.hidden = false));
    })
    .catch(() => {}); // without it, the page simply has no demo button
}

// The page is written in English; each text's Japanese is in its data-ja
// (HTML, ours). The English is kept in data-en to switch back.
function translate() {
  const ja = language() === 'ja';
  document.querySelectorAll<HTMLElement>('[data-ja]').forEach((el) => {
    el.dataset.en ??= el.innerHTML;
    el.innerHTML = ja ? el.dataset.ja! : el.dataset.en;
  });
  const description = document.querySelector<HTMLMetaElement>('meta[data-ja-content]');
  if (description) {
    description.dataset.en ??= description.content;
    description.content = ja ? description.dataset.jaContent! : description.dataset.en;
  }
  // The switch names the other language, in that language.
  const toggle = document.querySelector<HTMLElement>('#language-toggle');
  if (toggle) toggle.lang = ja ? 'en' : 'ja';
}

// Sections fade up as they come into view, once each. With reduced motion
// (or no IntersectionObserver) nothing is hidden in the first place.
function revealOnScroll() {
  if (matchMedia('(prefers-reduced-motion: reduce)').matches || !('IntersectionObserver' in window)) return;
  document.documentElement.classList.add('motion');
  const seen = new IntersectionObserver(
    (entries) => {
      for (const e of entries) {
        if (!e.isIntersecting) continue;
        e.target.classList.add('in');
        seen.unobserve(e.target);
      }
    },
    { rootMargin: '0px 0px -12% 0px' },
  );
  document.querySelectorAll('.reveal').forEach((el) => seen.observe(el));
}
