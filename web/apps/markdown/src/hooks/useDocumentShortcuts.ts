import type { DocumentResponse, ListResponse, MdFile, Route } from '../types';
import { useKeyboardShortcuts } from './useKeyboardShortcuts';

interface UseDocumentShortcutsOptions {
  route: Route;
  doc: DocumentResponse | null;
  list: ListResponse | null;
  files: MdFile[];
  navigate: (href: string) => void;
  onSave: () => void;
}

export function useDocumentShortcuts({ route, doc, list, files, navigate, onSave }: UseDocumentShortcutsOptions) {
  useKeyboardShortcuts({
    onEdit: () => {
      if (route.mode !== 'edit' && doc && !list?.contentMode) navigate('/edit' + doc.filePath);
    },
    onSave: () => {
      if (route.mode === 'edit') onSave();
    },
    onEscape: () => {
      if (route.mode === 'edit' && doc) navigate('/view' + doc.filePath);
    },
    onPrev: () => navigateSibling(files, doc?.filePath, -1, navigate),
    onNext: () => navigateSibling(files, doc?.filePath, 1, navigate),
  });
}

function navigateSibling(files: MdFile[], current: string | undefined, offset: -1 | 1, navigate: (href: string) => void) {
  const idx = files.findIndex(file => file.Relative === current);
  const next = idx + offset;
  if (idx >= 0 && next >= 0 && next < files.length) navigate('/view' + files[next].Relative);
}
