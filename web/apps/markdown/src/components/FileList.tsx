import { useMemo, useState } from 'react';
import { Tree } from 'antd';
import type { DataNode } from 'antd/es/tree';
import { Empty } from 'antd';
import {
  FileMarkdownOutlined,
  FolderOutlined,
} from '@ant-design/icons';
import type { MdFile } from '../types';

interface FileListProps {
  files: MdFile[];
  current: string;
  navigate: (href: string) => void;
}

interface FileTreeNode {
  key: string;
  title: React.ReactNode;
  file?: MdFile;
  childrenMap: Map<string, FileTreeNode>;
}

function buildTree(files: MdFile[]): DataNode[] {
  const root = new Map<string, FileTreeNode>();

  for (const file of files) {
    const parts = file.Relative.replace(/^\//, '').split('/').filter(Boolean);
    let level = root;
    let prefix = '';

    parts.forEach((part, index) => {
      const isFile = index === parts.length - 1;
      const key = isFile ? file.Relative : `dir:${prefix}${part}`;

      if (!level.has(part)) {
        level.set(part, {
          key,
          title: part,
          childrenMap: new Map(),
          file: isFile ? file : undefined,
        });
      }

      const node = level.get(part)!;
      if (isFile) node.file = file;
      level = node.childrenMap;
      prefix += part + '/';
    });
  }

  const convert = (m: Map<string, FileTreeNode>): DataNode[] =>
    [...m.values()]
      .sort((a, b) => String(a.title).localeCompare(String(b.title)))
      .map(n =>
        n.file
          ? {
              key: n.file.Relative,
              title: (
                <span className="tree-file-title">
                  {n.file.Title || n.file.Name}
                  {n.file.HasFrontmatter && <em>FM</em>}
                </span>
              ),
              icon: <FileMarkdownOutlined />,
            }
          : {
              key: n.key,
              title: n.title,
              icon: <FolderOutlined />,
              children: convert(n.childrenMap),
            }
      );

  return convert(root);
}

export function FileTree({ files, current, navigate }: FileListProps) {
  const [expandedKeys, setExpandedKeys] = useState<React.Key[]>(() =>
    JSON.parse(localStorage.getItem('md-tree-expanded') || '[]')
  );

  const treeData = useMemo(() => buildTree(files), [files]);

  return (
    <Tree
      className="md-ant-tree"
      blockNode
      treeData={treeData}
      selectedKeys={current ? [current] : []}
      expandedKeys={expandedKeys}
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

export function FileList({ files, current, navigate, mode }: FileListProps & { mode: 'tree' | 'flat' }) {
  if (!files.length) return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="No markdown files" />;

  if (mode === 'flat') {
    return (
      <ul className="file-list">
        {files.map(f => (
          <li key={f.Relative} className="file-item">
            <button
              className={`file-link ${current === f.Relative ? 'active' : ''}`}
              onClick={() => navigate('/view' + f.Relative)}
              type="button"
            >
              <b>{f.Title || f.Name}</b>
              <span>{f.Relative}</span>
              {f.HasFrontmatter && <em>FM</em>}
            </button>
          </li>
        ))}
      </ul>
    );
  }

  return <FileTree files={files} current={current} navigate={navigate} />;
}
