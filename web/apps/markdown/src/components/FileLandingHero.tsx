import { Input, Segmented } from 'antd';
import { AppstoreOutlined, UnorderedListOutlined } from '@ant-design/icons';
import type { LandingView } from './FileLandingResults';

interface FileLandingHeroProps {
  workspaceName?: string;
  total: number;
  query: string;
  setQuery: (query: string) => void;
  view: LandingView;
  setView: (view: LandingView) => void;
}

export function FileLandingHero({ workspaceName, total, query, setQuery, view, setView }: FileLandingHeroProps) {
  return (
    <section className="files-home-hero">
      <div>
        <h1>{workspaceName || 'Files'}</h1>
        <p>{total ? `${total} markdown files found` : 'No .md files found in the current directory.'}</p>
      </div>
      <div className="files-home-actions">
        <Input.Search
          allowClear
          placeholder="Search title, path or description"
          value={query}
          onChange={event => setQuery(event.target.value)}
        />
        <Segmented
          value={view}
          onChange={value => setView(value as LandingView)}
          options={[
            { label: 'Card', value: 'card', icon: <AppstoreOutlined /> },
            { label: 'List', value: 'list', icon: <UnorderedListOutlined /> },
          ]}
        />
      </div>
    </section>
  );
}
