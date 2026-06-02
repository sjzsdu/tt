import { useMemo, useState, type Key } from 'react';
import { Tree } from 'antd';
import type { MdFile } from '../types';
import { buildFileTree, dirKeysForFiles } from '../utils/fileTree';

interface FileTreeProps {
  files: MdFile[];
  current: string;
  navigate: (href: string) => void;
  searchActive?: boolean;
}

function readExpandedKeys() {
  try {
    return JSON.parse(localStorage.getItem('md-tree-expanded') || '[]') as Key[];
  } catch {
    return [];
  }
}

export function FileTree({ files, current, navigate, searchActive }: FileTreeProps) {
  const [expandedKeys, setExpandedKeys] = useState<Key[]>(readExpandedKeys);
  const treeData = useMemo(() => buildFileTree(files), [files]);
  const searchExpandedKeys = useMemo(() => dirKeysForFiles(files), [files]);

  return (
    <Tree
      className="md-ant-tree"
      blockNode
      treeData={treeData}
      selectedKeys={current ? [current] : []}
      expandedKeys={searchActive ? searchExpandedKeys : expandedKeys}
      onExpand={keys => {
        setExpandedKeys(keys);
        localStorage.setItem('md-tree-expanded', JSON.stringify(keys));
      }}
      onSelect={(_, info) => {
        const path = String(info.node.key);
        if (path.startsWith('/')) navigate('/view' + path);
      }}
    />
  );
}
