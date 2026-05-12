import { useEffect, useRef, useState } from 'react';
import { useMermaid } from '../hooks/useMermaid';
import { usePanZoom } from '../hooks/usePanZoom';
import { svgToBlob, svgMarkupToPngBlob, downloadBlob } from '../utils/svgExport';
import { MermaidToolbar } from './MermaidToolbar';

interface MermaidFigureProps {
  code: string;
  index: number;
}

interface SvgSize {
  minX: number;
  minY: number;
  width: number;
  height: number;
}

function transformedBox(el: SVGGraphicsElement): SvgSize | null {
  const box = el.getBBox();
  const matrix = el.getCTM();
  if (!matrix || box.width <= 0 || box.height <= 0) return null;

  const points = [
    new DOMPoint(box.x, box.y),
    new DOMPoint(box.x + box.width, box.y),
    new DOMPoint(box.x, box.y + box.height),
    new DOMPoint(box.x + box.width, box.y + box.height),
  ].map(point => point.matrixTransform(matrix));

  const xs = points.map(point => point.x);
  const ys = points.map(point => point.y);
  const minX = Math.min(...xs);
  const maxX = Math.max(...xs);
  const minY = Math.min(...ys);
  const maxY = Math.max(...ys);
  if (maxX <= minX || maxY <= minY) return null;

  return {
    minX,
    minY,
    width: maxX - minX,
    height: maxY - minY,
  };
}

function readSvgSize(svg: SVGSVGElement): SvgSize {
  try {
    const contentEl = svg.querySelector<SVGGraphicsElement>('g') || svg;
    const box = transformedBox(contentEl);
    if (box) {
      const padding = 12;
      return {
        minX: Math.floor(box.minX - padding),
        minY: Math.floor(box.minY - padding),
        width: Math.ceil(box.width + padding * 2),
        height: Math.ceil(box.height + padding * 2),
      };
    }
  } catch {
    // getBBox may fail for detached or not-yet-painted SVGs. Fall back below.
  }

  const viewBox = svg.viewBox?.baseVal;
  if (viewBox && viewBox.width > 0 && viewBox.height > 0) {
    return {
      minX: viewBox.x,
      minY: viewBox.y,
      width: Math.ceil(viewBox.width),
      height: Math.ceil(viewBox.height),
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
  const viewportRef = useRef<HTMLDivElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const svgElRef = useRef<SVGSVGElement | null>(null);
  const { svg, err } = useMermaid(code, index);
  const [panZoomTarget, setPanZoomTarget] = useState<HTMLDivElement | null>(null);
  const [baseSize, setBaseSize] = useState<SvgSize>({ minX: 0, minY: 0, width: 600, height: 300 });

  useEffect(() => {
    if (!svg || !containerRef.current) return;
    const el = containerRef.current.querySelector<SVGSVGElement>('svg');
    if (!el) return;

    const nextBaseSize = readSvgSize(el);
    const originalViewBox = `${nextBaseSize.minX} ${nextBaseSize.minY} ${nextBaseSize.width} ${nextBaseSize.height}`;

    svgElRef.current = el;
    el.setAttribute('viewBox', originalViewBox);
    el.setAttribute('width', String(nextBaseSize.width));
    el.setAttribute('height', String(nextBaseSize.height));
    el.dataset.originalViewBox = originalViewBox;
    el.dataset.exportWidth = String(nextBaseSize.width);
    el.dataset.exportHeight = String(nextBaseSize.height);
    el.style.width = `${nextBaseSize.width}px`;
    el.style.height = `${nextBaseSize.height}px`;
    el.style.maxWidth = 'none';
    el.style.display = 'block';

    setBaseSize(nextBaseSize);
    setPanZoomTarget(containerRef.current);
  }, [svg]);

  const panZoom = usePanZoom(panZoomTarget);

  useEffect(() => {
    const eventTarget = viewportRef.current;
    if (!eventTarget) return;
    eventTarget.addEventListener('wheel', panZoom.onWheel, { passive: false });
    eventTarget.addEventListener('pointerdown', panZoom.onPointerDown);
    eventTarget.addEventListener('pointermove', panZoom.onPointerMove);
    eventTarget.addEventListener('pointerup', panZoom.onPointerUp);
    eventTarget.addEventListener('pointercancel', panZoom.onPointerUp);
    eventTarget.addEventListener('lostpointercapture', panZoom.onPointerUp);
    return () => {
      eventTarget.removeEventListener('wheel', panZoom.onWheel);
      eventTarget.removeEventListener('pointerdown', panZoom.onPointerDown);
      eventTarget.removeEventListener('pointermove', panZoom.onPointerMove);
      eventTarget.removeEventListener('pointerup', panZoom.onPointerUp);
      eventTarget.removeEventListener('pointercancel', panZoom.onPointerUp);
      eventTarget.removeEventListener('lostpointercapture', panZoom.onPointerUp);
    };
  }, [panZoom]);

  const handleExportSvg = () => {
    if (svgElRef.current) {
      downloadBlob(svgToBlob(svgElRef.current), 'mermaid-diagram.svg');
    }
  };

  const handleExportPng = async () => {
    if (svg) {
      downloadBlob(await svgMarkupToPngBlob(svg), 'mermaid-diagram.png');
    }
  };

  const handleCopy = async () => {
    if (svg) {
      const blob = await svgMarkupToPngBlob(svg);
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
        ref={viewportRef}
        style={{ height: Math.max(80, baseSize.height) }}
      >
        <div
          className="mermaid-stage"
          style={{
            width: Math.ceil(baseSize.width * panZoom.scale),
            height: Math.ceil(baseSize.height * panZoom.scale),
          }}
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
      </div>
    </section>
  );
}
