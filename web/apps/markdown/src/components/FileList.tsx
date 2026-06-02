import { Empty } from 'antd';
import type { MdFile } from '../types';
import { FileTree } from './FileTree';
import { FlatFileList } from './FlatFileList';

interface FileListProps {
  files: MdFile[];
  current: string;
  navigate: (href: string) => void;
  mode: 'tree' | 'flat';
  searchActive?: boolean;
}

export function FileList({ files, current, navigate, mode, searchActive }: FileListProps) {
  if (!files.length) return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="No markdown files" />;
  if (mode === 'flat') return <FlatFileList files={files} current={current} navigate={navigate} />;
  return <FileTree files={files} current={current} navigate={navigate} searchActive={searchActive} />;
}
