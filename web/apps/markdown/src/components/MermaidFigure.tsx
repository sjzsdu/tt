import { useEffect, useRef, useState } from 'react';
import { useMermaid } from '../hooks/useMermaid';
import { usePanZoom } from '../hooks/usePanZoom';
import { svgToBlob, svgToPngBlob, downloadBlob } from '../utils/svgExport';
import { MermaidToolbar } from './MermaidToolbar';

interface MermaidFigureProps {
  code: string;
  index: number;
}

function readSvgSize(svg: SVGSVGElement) {
  const viewBox = svg.viewBox?.baseVal;
  if (viewBox && viewBox.width > 0 && viewBox.height > 0) {
    return {
      minX: viewBox.x,
      minY: viewBox.y,
      width: viewBox.width,
      height: viewBox.height,
    };
  }

  const rect = svg.getBoundingClientRect();
  const width = Number(svg.getAttribute('width')) || rect.width || 600;
  const height = Number(svg.getAttribute('height')) || rect.height || 300;
  return {
    minX: 0,
    minY: 0,
    width: Math.max(1, Math.ceil(width)),
    height: Math.max(1, Math.ceil(height)),
  };
}

export function MermaidFigure({ code, index }: MermaidFigureProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const svgElRef = useRef<SVGSVGElement | null>(null);
  const { svg, err } = useMermaid(code, index);
  const [svgEl, setSvgEl] = useState<SVGSVGElement | null>(null);
  const [baseSize, setBaseSize] = useState({ minX: 0, minY: 0, width: 600, height: 300 });

  useEffect(() => {
    if (!svg || !containerRef.current) return;
    const el = containerRef.current.querySelector<SVGSVGElement>('svg');
    if (!el) return;

    const nextBaseSize = readSvgSize(el);
    const originalViewBox = `${nextBaseSize.minX} ${nextBaseSize.minY} ${nextBaseSize.width} ${nextBaseSize.height}`;

    svgElRef.current = el;
    el.setAttribute('viewBox', originalViewBox);
    el.dataset.originalViewBox = originalViewBox;
    el.dataset.exportWidth = String(Math.ceil(nextBaseSize.width));
    el.dataset.exportHeight = String(Math.ceil(nextBaseSize.height));
    el.style.cursor = 'grab';
    el.style.transform = '';
    el.style.transformOrigin = '';

    setBaseSize(nextBaseSize);
    setSvgEl(el);
  }, [svg]);

  const panZoom = usePanZoom(svgEl, baseSize);

  useEffect(() => {
    if (!svgEl) return;
    svgEl.addEventListener('wheel', panZoom.onWheel, { passive: false });
    svgEl.addEventListener('pointerdown', panZoom.onPointerDown);
    svgEl.addEventListener('pointermove', panZoom.onPointerMove);
    svgEl.addEventListener('pointerup', panZoom.onPointerUp);
    svgEl.addEventListener('pointercancel', panZoom.onPointerUp);
    svgEl.addEventListener('lostpointercapture', panZoom.onPointerUp);
    return () => {
      svgEl.removeEventListener('wheel', panZoom.onWheel);
      svgEl.removeEventListener('pointerdown', panZoom.onPointerDown);
      svgEl.removeEventListener('pointermove', panZoom.onPointerMove);
      svgEl.removeEventListener('pointerup', panZoom.onPointerUp);
      svgEl.removeEventListener('pointercancel', panZoom.onPointerUp);
      svgEl.removeEventListener('lostpointercapture', panZoom.onPointerUp);
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
        style={{ minHeight: Math.max(180, baseSize.height + 36) }}
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
