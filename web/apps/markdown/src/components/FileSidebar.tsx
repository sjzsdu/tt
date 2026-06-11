import { MenuFoldOutlined } from '@ant-design/icons';
import { Input, Segmented, Tooltip } from 'antd';
import type { MdFile } from '../types';
import { filterFiles } from '../utils/fileSearch';
import { FileList } from './FileList';

interface FileSidebarProps {
  theme: 'light' | 'dark';
  onThemeChange: (theme: 'light' | 'dark') => void;
  files: MdFile[];
  current: string;
  navigate: (href: string) => void;
  fileMode: 'tree' | 'flat';
  setFileMode: (mode: 'tree' | 'flat') => void;
  fileQuery: string;
  setFileQuery: (query: string) => void;
  onCollapse?: () => void;
}

export function FileSidebar({
  theme,
  onThemeChange,
  files,
  current,
  navigate,
  fileMode,
  setFileMode,
  fileQuery,
  setFileQuery,
  onCollapse,
}: FileSidebarProps) {
  const filteredFiles = filterFiles(files, fileQuery);
  return (
    <aside className="files-pane section">
      <div className="section-heading">
        <div>
          <h1 className="section-title">Markdown Files</h1>
          <p className="section-subtitle">Browse and edit local Markdown files.</p>
        </div>
        {onCollapse && (
          <Tooltip title="Collapse file list">
            <button
              className="sidebar-collapse-btn"
              type="button"
              onClick={onCollapse}
              aria-label="Collapse file list"
            >
              <MenuFoldOutlined />
            </button>
          </Tooltip>
        )}
      </div>
      <Input.Search
        className="file-search"
        allowClear
        size="small"
        placeholder="Search files"
        value={fileQuery}
        onChange={event => setFileQuery(event.target.value)}
      />
      <div className="file-toolbar">
        <span>{filteredFiles.length}/{files.length} files</span>
        <div className="file-toolbar-actions">
          <Tooltip title="Switch theme">
            <Segmented
              size="small"
              value={theme}
              onChange={value => onThemeChange(value as 'light' | 'dark')}
              options={[
                { label: 'Light', value: 'light' },
                { label: 'Dark', value: 'dark' },
              ]}
            />
          </Tooltip>
          <Segmented
            size="small"
            value={fileMode}
            onChange={value => setFileMode(value as 'tree' | 'flat')}
            options={[
              { label: 'Tree', value: 'tree' },
              { label: 'List', value: 'flat' },
            ]}
          />
        </div>
      </div>
      <FileList files={filteredFiles} current={current} navigate={navigate} mode={fileMode} searchActive={Boolean(fileQuery.trim())} />
    </aside>
  );
}
