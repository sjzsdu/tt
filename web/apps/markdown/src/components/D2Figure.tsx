import { useRef } from 'react';
import { useD2 } from '../hooks/useD2';
import { useD2Actions } from '../hooks/useD2Actions';
import { useD2Viewport } from '../hooks/useD2Viewport';
import { D2Toolbar } from './D2Toolbar';

interface D2FigureProps {
  code: string;
  index: number;
  theme?: 'light' | 'dark';
}

function D2Body({ err, svg }: { err: string; svg: string }) {
  if (err) {
    return (
      <div className="d2-error">
        <div className="d2-error-title">Render failed</div>
        <pre>{err}</pre>
      </div>
    );
  }

  if (!svg) {
    return (
      <div className="d2-loading">
        <span>Rendering D2 diagram...</span>
      </div>
    );
  }

  return <div className="d2-svg" dangerouslySetInnerHTML={{ __html: svg }} />;
}

export function D2Figure({ code, index, theme = ((document.documentElement.dataset.theme as 'light' | 'dark') || 'light') }: D2FigureProps) {
  const figureRef = useRef<HTMLElement>(null);
  const { svg, err } = useD2(code, index, theme);
  const { containerRef, svgElRef, displaySvg, baseSize } = useD2Viewport(svg);
  const actions = useD2Actions(svg, svgElRef);
  const subtitle = err
    ? 'Render failed'
    : baseSize
      ? `${Math.round(baseSize.width)}×${Math.round(baseSize.height)} · scroll to inspect`
      : svg
        ? 'Rendered from D2'
        : 'Rendering...';

  return (
    <figure className="d2-figure" ref={figureRef}>
      <D2Toolbar
        subtitle={subtitle}
        onExportSvg={actions.exportSvg}
        onExportPng={actions.exportPng}
        onCopy={actions.copyPng}
      />
      <div className="d2-viewport">
        <div ref={containerRef} className="d2-canvas">
          <D2Body err={err} svg={displaySvg} />
        </div>
      </div>
    </figure>
  );
}
