import type { ReactNode } from 'react';
import { FileMarkdownOutlined, FolderOutlined, TagOutlined } from '@ant-design/icons';
import type { DataNode } from 'antd/es/tree';
import type { MdFile } from '../types';

interface FileTreeNode {
  key: string;
  title: ReactNode;
  file?: MdFile;
  childrenMap: Map<string, FileTreeNode>;
}

export function buildFileTree(files: MdFile[]): DataNode[] {
  const root = new Map<string, FileTreeNode>();

  for (const file of files) {
    const parts = file.Relative.replace(/^\//, '').split('/').filter(Boolean);
    let level = root;
    let prefix = '';

    parts.forEach((part, index) => {
      const isFile = index === parts.length - 1;
      const key = isFile ? file.Relative : `dir:${prefix}${part}`;

      if (!level.has(part)) {
        level.set(part, { key, title: part, childrenMap: new Map(), file: isFile ? file : undefined });
      }

      const node = level.get(part)!;
      if (isFile) node.file = file;
      level = node.childrenMap;
      prefix += part + '/';
    });
  }

  return convertTreeNodes(root);
}

export function dirKeysForFiles(files: MdFile[]) {
  const keys = new Set<string>();
  for (const file of files) {
    const parts = file.Relative.replace(/^\//, '').split('/').filter(Boolean);
    let prefix = '';
    parts.slice(0, -1).forEach(part => {
      keys.add(`dir:${prefix}${part}`);
      prefix += part + '/';
    });
  }
  return [...keys];
}

function convertTreeNodes(nodes: Map<string, FileTreeNode>): DataNode[] {
  return [...nodes.values()]
    .sort((a, b) => String(a.title).localeCompare(String(b.title)))
    .map(node => node.file ? fileNode(node.file) : directoryNode(node));
}

function fileNode(file: MdFile): DataNode {
  return {
    key: file.Relative,
    title: (
      <span className="tree-file-title">
        {file.Title || file.Name}
        {file.HasFrontmatter && <TagOutlined className="fm-icon" />}
      </span>
    ),
    icon: <FileMarkdownOutlined />,
  };
}

function directoryNode(node: FileTreeNode): DataNode {
  return {
    key: node.key,
    title: node.title,
    icon: <FolderOutlined />,
    children: convertTreeNodes(node.childrenMap),
  };
}
