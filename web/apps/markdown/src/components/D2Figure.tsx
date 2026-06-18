import { useRef } from 'react';
import { useD2 } from '../hooks/useD2';
import { useMermaidActions } from '../hooks/useMermaidActions';
import { useMermaidViewport } from '../hooks/useMermaidViewport';
import { usePanZoom } from '../hooks/usePanZoom';
import { MermaidToolbar } from './MermaidToolbar';

interface D2FigureProps {
  code: string;
  index: number;
  theme?: 'light' | 'dark';
}

function D2Body({ err, svg }: { err: string; svg: string }) {
  if (err) return <pre className="mermaid-error">{err}</pre>;
  if (!svg) {
    return (
      <div className="mermaid-loading">
        <span>Rendering D2 diagram...</span>
      </div>
    );
  }
  return <div className="mermaid-svg" dangerouslySetInnerHTML={{ __html: svg }} />;
}

export function D2Figure({ code, index, theme = ((document.documentElement.dataset.theme as 'light' | 'dark') || 'light') }: D2FigureProps) {
  const figureRef = useRef<HTMLElement>(null);
  const { svg, err } = useD2(code, index, theme);
  const {
    viewportRef,
    containerRef,
    svgElRef,
    panZoomTarget,
    initialState,
    viewportHeight,
    displaySvg,
  } = useMermaidViewport(svg);
  const actions = useMermaidActions(svg, svgElRef, 'd2-diagram');
  const { scale, zoomIn, zoomOut, reset, setScale } = usePanZoom(panZoomTarget, initialState);

  return (
    <figure className="mermaid-figure d2-figure" ref={figureRef}>
      <MermaidToolbar
        title="D2 diagram"
        subtitle={err ? 'Render failed' : svg ? 'Rendered from D2' : 'Rendering...'}
        scale={scale}
        onZoomIn={zoomIn}
        onZoomOut={zoomOut}
        onReset={reset}
        onScaleChange={setScale}
        onExportSvg={actions.exportSvg}
        onExportPng={actions.exportPng}
        onCopy={actions.copyPng}
      />
      <div className="mermaid-viewport" ref={viewportRef} style={{ height: viewportHeight }}>
        {svg && !err ? (
          <div ref={containerRef} className="mermaid-panzoom-target">
            <D2Body err={err} svg={displaySvg} />
          </div>
        ) : (
          <D2Body err={err} svg={displaySvg} />
        )}
      </div>
    </figure>
  );
}
