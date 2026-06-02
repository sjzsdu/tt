import type { Route } from '../types';

export function currentRoute(): Route {
  const path = window.location.pathname;
  if (path.startsWith('/edit/')) return { mode: 'edit', file: path.slice('/edit'.length) };
  if (path.startsWith('/view/')) return { mode: 'view', file: path.slice('/view'.length) };
  return { mode: 'list', file: '' };
}
