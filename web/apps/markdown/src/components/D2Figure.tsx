import { useRef } from 'react';
import { useD2 } from '../hooks/useD2';
import { useD2Actions } from '../hooks/useD2Actions';
import { useD2Viewport } from '../hooks/useD2Viewport';
import { usePanZoom } from '../hooks/usePanZoom';
import { usePanZoomEvents } from '../hooks/usePanZoomEvents';
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

  return <div className="d2-svg-wrapper" dangerouslySetInnerHTML={{ __html: svg }} />;
}

export function D2Figure({ code, index, theme = ((document.documentElement.dataset.theme as 'light' | 'dark') || 'light') }: D2FigureProps) {
  const figureRef = useRef<HTMLElement>(null);
  const { svg, err } = useD2(code, index, theme);
  const {
    viewportRef,
    containerRef,
    svgElRef,
    panZoomTarget,
    displaySvg,
    baseSize,
    initialState,
    viewportHeight,
  } = useD2Viewport(svg);
  const panZoom = usePanZoom(panZoomTarget, { initialState, minScale: 0.08 });
  const actions = useD2Actions(svg, svgElRef);

  usePanZoomEvents(viewportRef, panZoom);

  const subtitle = err
    ? 'Render failed'
    : svg
      ? `${Math.round(baseSize.width)}×${Math.round(baseSize.height)} · drag to pan · wheel to zoom`
      : 'Rendering...';

  return (
    <figure className="d2-figure" ref={figureRef}>
      <D2Toolbar
        scale={panZoom.scale}
        subtitle={subtitle}
        onZoomIn={panZoom.zoomIn}
        onZoomOut={panZoom.zoomOut}
        onReset={panZoom.reset}
        onScaleChange={panZoom.setScale}
        onExportSvg={actions.exportSvg}
        onExportPng={actions.exportPng}
        onCopy={actions.copyPng}
      />
      <div className="d2-viewport" ref={viewportRef} style={{ height: viewportHeight }}>
        <div className="d2-stage" style={{ width: baseSize.width, height: baseSize.height }}>
          <div ref={containerRef} className="d2-canvas">
            <D2Body err={err} svg={displaySvg} />
          </div>
        </div>
      </div>
    </figure>
  );
}
