import type { AppTheme, SlideData, SlideWidgetRegistry } from '../types';
import { MermaidBlock } from './MermaidBlock';
import { D2Block } from './D2Block';
import { renderWidgetFallback, renderWidgetTemplate, safeWidgetClassName } from '../widgets/render';

export function SlideContent({ slide, theme = 'dark', widgets = {} }: { slide: SlideData; theme?: AppTheme; widgets?: SlideWidgetRegistry }) {
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
          const block = <MermaidBlock code={part.code} index={slide.index * 100 + i} theme={theme} />;
          return part.role ? <div key={i} className={`slide-part-${part.role}`}>{block}</div> : <div key={i}>{block}</div>;
        }
        if (part.type === 'd2') {
          const block = <D2Block code={part.code} index={slide.index * 100 + i} theme={theme} />;
          return part.role ? <div key={i} className={`slide-part-${part.role}`}>{block}</div> : <div key={i}>{block}</div>;
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
              dangerouslySetInnerHTML={{ __html: html }}
            />
          );
        }
        return null;
      })}
    </div>
  );
}
