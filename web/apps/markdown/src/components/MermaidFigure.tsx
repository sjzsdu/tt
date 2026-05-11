import { useEffect, useRef, useState } from 'react';
import { useMermaid } from '../hooks/useMermaid';
import { usePanZoom } from '../hooks/usePanZoom';
import { svgToBlob, svgToPngBlob, downloadBlob } from '../utils/svgExport';
import { MermaidToolbar } from './MermaidToolbar';

interface MermaidFigureProps {
  code: string;
  index: number;
}

export function MermaidFigure({ code, index }: MermaidFigureProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const svgElRef = useRef<SVGSVGElement | null>(null);
  const baseSizeRef = useRef({ width: 600, height: 300 });
  const { svg, err } = useMermaid(code, index);
  const [svgEl, setSvgEl] = useState<SVGSVGElement | null>(null);

  useEffect(() => {
    if (!svg || !containerRef.current) return;
    const el = containerRef.current.querySelector<SVGSVGElement>('svg');
    if (!el) return;

    svgElRef.current = el;
    const rect = el.getBoundingClientRect();
    baseSizeRef.current = {
      width: Math.max(1, Math.ceil(rect.width || Number(el.getAttribute('width')) || 600)),
      height: Math.max(1, Math.ceil(rect.height || Number(el.getAttribute('height')) || 300)),
    };

    const viewBox = el.getAttribute('viewBox');
    if (!viewBox) {
      el.setAttribute('viewBox', `0 0 ${baseSizeRef.current.width} ${baseSizeRef.current.height}`);
    }
    el.style.cursor = 'grab';
    el.removeAttribute('style');
    setSvgEl(el);
  }, [svg]);

  const panZoom = usePanZoom(svgEl, baseSizeRef.current.width, baseSizeRef.current.height);

  useEffect(() => {
    if (!svgEl) return;
    svgEl.addEventListener('wheel', panZoom.onWheel, { passive: false });
    svgEl.addEventListener('pointerdown', panZoom.onPointerDown);
    svgEl.addEventListener('pointermove', panZoom.onPointerMove);
    svgEl.addEventListener('pointerup', panZoom.onPointerUp);
    svgEl.addEventListener('pointercancel', panZoom.onPointerUp);
    return () => {
      svgEl.removeEventListener('wheel', panZoom.onWheel);
      svgEl.removeEventListener('pointerdown', panZoom.onPointerDown);
      svgEl.removeEventListener('pointermove', panZoom.onPointerMove);
      svgEl.removeEventListener('pointerup', panZoom.onPointerUp);
      svgEl.removeEventListener('pointercancel', panZoom.onPointerUp);
    };
  }, [svgEl, panZoom]);

  const handleExportSvg = () => {
    if (svgElRef.current) {
      downloadBlob(svgToBlob(svgElRef.current), 'mermaid-diagram.svg');
    }
  };

  const handleExportPng = async () => {
    if (svgElRef.current) {
      downloadBlob(await svgToPngBlob(svgElRef.current), 'mermaid-diagram.png');
    }
  };

  const handleCopy = async () => {
    if (svgElRef.current) {
      const blob = await svgToPngBlob(svgElRef.current);
      await navigator.clipboard.write([
        new ClipboardItem({ 'image/png': blob }),
      ]);
    }
  };

  return (
    <section className="mermaid-figure">
      <MermaidToolbar
        scale={panZoom.scale}
        onZoomIn={panZoom.zoomIn}
        onZoomOut={panZoom.zoomOut}
        onReset={panZoom.reset}
        onScaleChange={panZoom.setScale}
        onExportSvg={handleExportSvg}
        onExportPng={handleExportPng}
        onCopy={handleCopy}
      />
      <div
        className="mermaid-viewport"
        style={{ minHeight: Math.max(180, baseSizeRef.current.height + 36) }}
      >
        <div className="mermaid" ref={containerRef}>
          {err ? (
            <pre className="mermaid-error">{err}</pre>
          ) : svg ? (
            <div dangerouslySetInnerHTML={{ __html: svg }} />
          ) : (
            'Rendering Mermaid...'
          )}
        </div>
      </div>
    </section>
  );
}
