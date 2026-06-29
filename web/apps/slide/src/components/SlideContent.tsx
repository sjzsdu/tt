import type { SlideData } from '../parser';
import { MermaidBlock } from './MermaidBlock';
import { D2Block } from './D2Block';

export function SlideContent({ slide }: { slide: SlideData }) {
  return (
    <div className="slide-content">
      {slide.parts.map((part, i) => {
        if (part.type === 'markdown') {
          return (
            <div
              key={i}
              className="slide-markdown"
              dangerouslySetInnerHTML={{ __html: part.html }}
            />
          );
        }
        if (part.type === 'mermaid') {
          return <MermaidBlock key={i} code={part.code} index={slide.index * 100 + i} />;
        }
        if (part.type === 'd2') {
          return <D2Block key={i} code={part.code} index={slide.index * 100 + i} />;
        }
        return null;
      })}
    </div>
  );
}
