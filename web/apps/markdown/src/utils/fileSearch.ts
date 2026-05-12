import type { MdFile } from '../types';

export function matchesFileQuery(file: MdFile, query: string) {
  const normalized = query.trim().toLowerCase();
  if (!normalized) return true;
  return [file.Title, file.Name, file.Relative, file.Description]
    .filter(Boolean)
    .join('\n')
    .toLowerCase()
    .includes(normalized);
}

export function filterFiles(files: MdFile[], query: string) {
  const normalized = query.trim();
  if (!normalized) return files;
  return files.filter(file => matchesFileQuery(file, normalized));
}
