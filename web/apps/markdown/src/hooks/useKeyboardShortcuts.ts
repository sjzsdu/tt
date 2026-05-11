import { useEffect } from 'react';

export function useKeyboardShortcuts({
  onEdit,
  onSave,
  onEscape,
  onPrev,
  onNext,
}: {
  onEdit?: () => void;
  onSave?: () => void;
  onEscape?: () => void;
  onPrev?: () => void;
  onNext?: () => void;
}) {
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      const tag = (e.target as HTMLElement).tagName;
      if (tag === 'TEXTAREA' || tag === 'INPUT') return;

      if (e.key === 'e' && !e.metaKey && !e.ctrlKey) {
        onEdit?.();
      } else if ((e.metaKey || e.ctrlKey) && e.key === 's') {
        e.preventDefault();
        onSave?.();
      } else if (e.key === 'Escape') {
        onEscape?.();
      } else if (e.key === 'ArrowLeft') {
        onPrev?.();
      } else if (e.key === 'ArrowRight') {
        onNext?.();
      }
    };
    addEventListener('keydown', handler);
    return () => removeEventListener('keydown', handler);
  }, [onEdit, onSave, onEscape, onPrev, onNext]);
}
