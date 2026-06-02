import { useRef } from 'react';
import { useMermaid } from '../hooks/useMermaid';
import { useMermaidActions } from '../hooks/useMermaidActions';
import { useMermaidViewport } from '../hooks/useMermaidViewport';
import { usePanZoom } from '../hooks/usePanZoom';
import { usePanZoomEvents } from '../hooks/usePanZoomEvents';
import { MermaidToolbar } from './MermaidToolbar';

interface MermaidFigureProps {
  code: string;
  index: number;
  theme?: 'light' | 'dark';
}

function diagramKind(code: string) {
  const first = code.trim().split(/\s+/)[0] || 'diagram';
  const labels: Record<string, string> = {
    flowchart: 'Flowchart',
    graph: 'Flowchart',
    sequenceDiagram: 'Sequence',
    classDiagram: 'Class diagram',
    stateDiagram: 'State diagram',
    erDiagram: 'ER diagram',
    gantt: 'Gantt',
    pie: 'Pie chart',
    journey: 'Journey',
    timeline: 'Timeline',
    mindmap: 'Mindmap',
  };
  return labels[first] || first.replace(/([a-z])([A-Z])/g, '$1 $2');
}

function MermaidBody({ err, svg }: { err: string; svg: string }) {
  if (err) {
    return (
      <div className="mermaid-error">
        <div className="mermaid-error-icon">️</div>
        <div className="mermaid-error-text">Render failed</div>
        <pre>{err}</pre>
      </div>
    );
  }

  if (svg) return <div dangerouslySetInnerHTML={{ __html: svg }} />;

  return (
    <div className="mermaid-loading">
      <div className="mermaid-loading-spinner" />
      <span>Rendering diagram...</span>
    </div>
  );
}

export function MermaidFigure({ code, index, theme = ((document.documentElement.dataset.theme as 'light' | 'dark') || 'light') }: MermaidFigureProps) {
  const figureRef = useRef<HTMLElement | null>(null);
  const { svg, err } = useMermaid(code, index, theme, figureRef);
  const {
    viewportRef,
    containerRef,
    svgElRef,
    panZoomTarget,
    baseSize,
    initialState,
    viewportHeight,
    displaySvg,
  } = useMermaidViewport(svg);
  const panZoom = usePanZoom(panZoomTarget, { initialState, minScale: 0.08 });
  const actions = useMermaidActions(svg, svgElRef);

  usePanZoomEvents(viewportRef, panZoom);

  return (
    <section className="mermaid-figure" ref={figureRef}>
      <MermaidToolbar
        scale={panZoom.scale}
        title={diagramKind(code)}
        subtitle={`${Math.round(baseSize.width)}×${Math.round(baseSize.height)} · drag to pan · wheel to zoom`}
        onZoomIn={panZoom.zoomIn}
        onZoomOut={panZoom.zoomOut}
        onReset={panZoom.reset}
        onScaleChange={panZoom.setScale}
        onExportSvg={actions.exportSvg}
        onExportPng={actions.exportPng}
        onCopy={actions.copyPng}
      />
      <div className="mermaid-viewport" ref={viewportRef} style={{ height: viewportHeight }}>
        <div className="mermaid-stage" style={{ width: baseSize.width, height: baseSize.height }}>
          <div className="mermaid-canvas" ref={containerRef}>
            <MermaidBody err={err} svg={displaySvg} />
          </div>
        </div>
      </div>
    </section>
  );
}
