import type { CSSProperties } from 'react';
import type { AppTheme, SlideData, SlideWidgetRegistry } from '../types';
import { MermaidBlock } from './MermaidBlock';
import { D2Block } from './D2Block';
import { renderWidgetFallback, renderWidgetTemplate, safeWidgetClassName } from '../widgets/render';

type SlidePart = SlideData['parts'][number];

function getNumberedClass(classes: string[], prefix: string): number | undefined {
  for (const className of classes) {
    const match = className.match(new RegExp(`^${prefix}-(\\d+)$`));
    if (!match) continue;
    const value = Number(match[1]);
    if (Number.isFinite(value) && value > 0) return value;
  }
  return undefined;
}

function getSlideContentStyle(classes: string[]): CSSProperties | undefined {
  if (!classes.includes('slide-grid')) return undefined;

  const cols = getNumberedClass(classes, 'slide-cols');
  const rows = getNumberedClass(classes, 'slide-rows');
  const style: CSSProperties = {
    display: 'grid',
    gridTemplateColumns: cols ? `repeat(${cols}, minmax(0, 1fr))` : 'repeat(auto-fit, minmax(300px, 1fr))',
    gap: classes.includes('slide-dense') ? 10 : classes.includes('slide-compact') ? 14 : 24,
    alignContent: rows ? 'stretch' : 'start',
    height: '100%',
  };

  if (rows) {
    style.gridTemplateRows = `auto repeat(${rows}, minmax(0, 1fr))`;
    style.alignItems = 'stretch';
  }

  return style;
}

function getPartStyle(classes: string[], part: SlidePart): CSSProperties | undefined {
  if (!classes.includes('slide-grid')) return undefined;

  if (part.type === 'markdown' && !part.role) {
    return { gridColumn: '1 / -1' };
  }

  if (part.type === 'widget' && (classes.includes('slide-dense') || classes.includes('slide-compact'))) {
    return { minWidth: 0, minHeight: 0, height: '100%' };
  }

  return undefined;
}

export function SlideContent({ slide, theme = 'dark', widgets = {} }: { slide: SlideData; theme?: AppTheme; widgets?: SlideWidgetRegistry }) {
  const slideClasses = slide.class.split(/\s+/).filter(Boolean);
  const contentStyle = getSlideContentStyle(slideClasses);

  return (
    <div className="slide-content" style={contentStyle}>
      {slide.parts.map((part, i) => {
        const partStyle = getPartStyle(slideClasses, part);
        if (part.type === 'markdown') {
          return (
            <div
              key={i}
              className={`slide-markdown ${part.role ? `slide-part-${part.role}` : ''}`}
              style={partStyle}
              dangerouslySetInnerHTML={{ __html: part.html }}
            />
          );
        }
        if (part.type === 'mermaid') {
          const block = <MermaidBlock code={part.code} index={slide.index * 100 + i} theme={theme} />;
          return part.role ? <div key={i} className={`slide-part-${part.role}`} style={partStyle}>{block}</div> : <div key={i} style={partStyle}>{block}</div>;
        }
        if (part.type === 'd2') {
          const block = <D2Block code={part.code} index={slide.index * 100 + i} theme={theme} />;
          return part.role ? <div key={i} className={`slide-part-${part.role}`} style={partStyle}>{block}</div> : <div key={i} style={partStyle}>{block}</div>;
        }
        if (part.type === 'widget') {
          const template = widgets[part.widgetType];
          const html = template ? renderWidgetTemplate(template.html, part.data) : renderWidgetFallback(part.widgetType, part.raw, widgets);
          const widgetClass = safeWidgetClassName(part.widgetType);
          return (
            <div
              key={i}
              className={`slide-widget slide-widget-${widgetClass} ${part.role ? `slide-part-${part.role}` : ''}`}
              data-widget-type={part.widgetType || 'unknown'}
              style={partStyle}
              dangerouslySetInnerHTML={{ __html: html }}
            />
          );
        }
        return null;
      })}
    </div>
  );
}
