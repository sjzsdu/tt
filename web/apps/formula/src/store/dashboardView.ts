import type { DashboardView } from '../types';

export function currentView(): DashboardView {
  return localStorage.getItem('formula-dashboard-view') === 'graph' ? 'graph' : 'list';
}

export function persistDashboardView(next: DashboardView) {
  localStorage.setItem('formula-dashboard-view', next);
}
