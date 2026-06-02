import { TagOutlined } from '@ant-design/icons';
import type { MdFile } from '../types';

interface FlatFileListProps {
  files: MdFile[];
  current: string;
  navigate: (href: string) => void;
}

export function FlatFileList({ files, current, navigate }: FlatFileListProps) {
  return (
    <ul className="file-list">
      {files.map(file => (
        <li key={file.Relative} className="file-item">
          <button
            className={`file-link ${current === file.Relative ? 'active' : ''}`}
            onClick={() => navigate('/view' + file.Relative)}
            type="button"
          >
            <b>{file.Title || file.Name}</b>
            <span>{file.Relative}</span>
            {file.HasFrontmatter && <TagOutlined className="fm-icon" />}
          </button>
        </li>
      ))}
    </ul>
  );
}
