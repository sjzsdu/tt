import { useRef } from 'react';

interface ScrollSnapshot {
  top: number;
  ratio: number;
}

export function useScrollMemory() {
  const contentPaneRef = useRef<HTMLElement | null>(null);
  const scrollPositionsRef = useRef<Record<string, ScrollSnapshot>>({});
  const activeScrollKeyRef = useRef('');

  const rememberCurrentScroll = () => {
    const key = activeScrollKeyRef.current;
    const pane = contentPaneRef.current;
    if (!key || !pane) return;
    const maxScrollTop = Math.max(0, pane.scrollHeight - pane.clientHeight);
    scrollPositionsRef.current[key] = {
      top: pane.scrollTop,
      ratio: maxScrollTop > 0 ? pane.scrollTop / maxScrollTop : 0,
    };
  };

  const restoreScrollPosition = (key: string) => {
    const pane = contentPaneRef.current;
    const snapshot = scrollPositionsRef.current[key];
    if (!pane || window.location.hash) return;
    requestAnimationFrame(() => {
      if (activeScrollKeyRef.current !== key) return;
      const currentPane = contentPaneRef.current;
      if (!currentPane) return;
      if (!snapshot) {
        currentPane.scrollTo({ top: 0, behavior: 'instant' });
        return;
      }
      const maxScrollTop = Math.max(0, currentPane.scrollHeight - currentPane.clientHeight);
      const ratioTop = snapshot.ratio * maxScrollTop;
      const targetTop = Math.min(maxScrollTop, Math.max(0, Math.round(Math.min(snapshot.top, ratioTop))));
      currentPane.scrollTo({ top: targetTop, behavior: 'instant' });
    });
  };

  const activateScrollKey = (key: string) => {
    activeScrollKeyRef.current = key;
    if (key) restoreScrollPosition(key);
  };

  return { contentPaneRef, rememberCurrentScroll, activateScrollKey };
}
