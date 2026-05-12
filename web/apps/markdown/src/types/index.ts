export type MdFile = {
  Path: string;
  Name: string;
  Size: number;
  Relative: string;
  Title: string;
  Description: string;
  HasFrontmatter: boolean;
  FrontmatterNum: number;
};

export type FrontmatterField = { Key: string; Value: string };

export type ListResponse = {
  files: MdFile[];
  total: number;
  contentMode: boolean;
  contentOnly: boolean;
  workspaceName?: string;
};

export type DocumentResponse = {
  filePath: string;
  rawPath: string;
  contentHTML: string;
  contentText: string;
  fullContent: string;
  files: MdFile[];
  editing: boolean;
  hasFrontmatter: boolean;
  frontmatterFields: FrontmatterField[];
  frontmatterRaw: string;
};

export type TocItem = { id: string; text: string; level: number };

export type Route = { mode: 'list' | 'view' | 'edit'; file: string };

export type MdPart = { type: 'markdown'; html: string } | { type: 'mermaid'; code: string };
