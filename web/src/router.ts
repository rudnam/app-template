// A tiny hash router: #/records?category=3. Hash routes need no server
// support and survive reloads.
import { useEffect, useState } from 'react';

export interface Route {
  page: string;
  params: URLSearchParams;
}

function parse(): Route {
  const [path, qs = ''] = location.hash.replace(/^#\/?/, '').split('?');
  return { page: path || 'home', params: new URLSearchParams(qs) };
}

export function useRoute(): Route {
  const [route, setRoute] = useState(parse);
  useEffect(() => {
    const on = () => {
      setRoute(parse());
      window.scrollTo(0, 0);
    };
    window.addEventListener('hashchange', on);
    return () => window.removeEventListener('hashchange', on);
  }, []);
  return route;
}

export function href(page: string, params?: Record<string, string | number>): string {
  const qs = params ? new URLSearchParams(Object.entries(params).map(([k, v]) => [k, String(v)])).toString() : '';
  return `#/${page}${qs ? `?${qs}` : ''}`;
}

export function navigate(page: string, params?: Record<string, string | number>) {
  location.hash = href(page, params);
}

/** navigate, replacing the current history entry, e.g. to drop a used link's token. */
export function replaceRoute(page: string, params?: Record<string, string | number>) {
  history.replaceState(null, '', href(page, params));
  window.dispatchEvent(new HashChangeEvent('hashchange'));
}
