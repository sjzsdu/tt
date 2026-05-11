import type { MdPart } from '../types';
import { MermaidFigure } from './MermaidFigure';

interface MarkdownContentProps {
  parts: MdPart[];
}

export function MarkdownContent({ parts }: MarkdownContentProps) {
  return (
    <>
      {parts.map((p, i) =>
        p.type === 'markdown' ? (
          <div
            key={i}
            className="markdown-chunk"
            dangerouslySetInnerHTML={{ __html: p.html }}
          />
        ) : (
          <MermaidFigure key={i} code={p.code} index={i} />
        )
      )}
    </>
  );
}
