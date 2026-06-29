import type { AppTheme, SlideData } from '../types';
import { MermaidBlock } from './MermaidBlock';
import { D2Block } from './D2Block';

export function SlideContent({ slide, theme = 'dark' }: { slide: SlideData; theme?: AppTheme }) {
  return (
    <div className="slide-content">
      {slide.parts.map((part, i) => {
        if (part.type === 'markdown') {
          return (
            <div
              key={i}
              className={`slide-markdown ${part.role ? `slide-part-${part.role}` : ''}`}
              dangerouslySetInnerHTML={{ __html: part.html }}
            />
          );
        }
        if (part.type === 'mermaid') {
          return <MermaidBlock key={i} code={part.code} index={slide.index * 100 + i} theme={theme} />;
        }
        if (part.type === 'd2') {
          return <D2Block key={i} code={part.code} index={slide.index * 100 + i} theme={theme} />;
        }
        return null;
      })}
    </div>
  );
}
