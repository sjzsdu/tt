import type { MdPart } from '../types';
import { D2Figure } from './D2Figure';
import { MermaidFigure } from './MermaidFigure';

interface MarkdownContentProps {
  parts: MdPart[];
  theme: 'light' | 'dark';
}

function slugify(text: string): string {
  return text
    .toLowerCase()
    .trim()
    .replace(/[^\w\u4e00-\u9fa5\s-]/g, '')
    .replace(/\s+/g, '-')
    .replace(/-+/g, '-');
}

function addIdsToHeadings(html: string): string {
  return html.replace(/<(h[1-4])([^>]*)>(.*?)<\/h[1-4]>/gi, (_, tag, attrs, content) => {
    const textMatch = content.replace(/<[^>]+>/g, '').trim();
    const text = textMatch || 'section';
    const base = slugify(text) || 'section';
    const id = base;
    return `<${tag}${attrs} id="${id}">${content}</${tag}>`;
  });
}

export function MarkdownContent({ parts, theme }: MarkdownContentProps) {
  return (
    <>
      {parts.map((p, i) =>
        p.type === 'markdown' ? (
          <div
            key={i}
            className="markdown-chunk"
            dangerouslySetInnerHTML={{ __html: addIdsToHeadings(p.html) }}
          />
        ) : p.type === 'mermaid' ? (
          <MermaidFigure key={i} code={p.code} index={i} theme={theme} />
        ) : (
          <D2Figure key={i} code={p.code} index={i} theme={theme} />
        )
      )}
    </>
  );
}
