import { useMemo } from 'react';
import type { ListResponse } from '../types';
import { usePersistentState } from '../hooks/usePersistentState';
import { filterFiles } from '../utils/fileSearch';
import { FileLandingHero } from './FileLandingHero';
import { FileLandingResults, type LandingView } from './FileLandingResults';

interface FileLandingProps {
  list: ListResponse | null;
  navigate: (href: string) => void;
  query: string;
  setQuery: (query: string) => void;
}

export function FileLanding({ list, navigate, query, setQuery }: FileLandingProps) {
  const [view, setView] = usePersistentState<LandingView>(
    'md-landing-view',
    'card',
    value => (value === 'list' ? 'list' : 'card')
  );
  const files = useMemo(() => filterFiles(list?.files || [], query), [list?.files, query]);

  if (!list) {
    return (
      <main className="files-home">
        <div className="empty">Loading...</div>
      </main>
    );
  }

  return (
    <main className="files-home">
      <FileLandingHero
        workspaceName={list.workspaceName}
        total={list.total}
        query={query}
        setQuery={setQuery}
        view={view}
        setView={setView}
      />
      <FileLandingResults allFiles={list.files} files={files} view={view} navigate={navigate} />
    </main>
  );
}
