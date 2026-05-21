import { useEffect, useRef, useState, useCallback } from 'react';
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

interface PanZoomState {
  scale: number;
  position: { x: number; y: number };
}

const MIN_VIEWPORT_HEIGHT = 300;

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
  const rect = svg.getBoundingClientRect();
  const viewBox = svg.viewBox?.baseVal;
  if (rect.width > 0 && rect.height > 0) {
    return {
      minX: viewBox?.x ?? 0,
      minY: viewBox?.y ?? 0,
      width: Math.max(1, Math.ceil(rect.width)),
      height: Math.max(1, Math.ceil(rect.height)),
    };
  }

  try {
    const box = svg.getBBox();
    if (box.width > 0 && box.height > 0) {
      const padding = 12;
      return {
        minX: Math.floor(box.x - padding),
        minY: Math.floor(box.y - padding),
        width: Math.ceil(box.width + padding * 2),
        height: Math.ceil(box.height + padding * 2),
      };
    }

    const contentEl = svg.querySelector<SVGGraphicsElement>('g');
    const transformed = contentEl ? transformedBox(contentEl) : null;
    if (transformed) {
      const padding = 12;
      return {
        minX: Math.floor(transformed.minX - padding),
        minY: Math.floor(transformed.minY - padding),
        width: Math.ceil(transformed.width + padding * 2),
        height: Math.ceil(transformed.height + padding * 2),
      };
    }
  } catch {
    // getBBox may fail for detached or not-yet-painted SVGs. Fall back below.
  }

  if (viewBox && viewBox.width > 0 && viewBox.height > 0) {
    return {
      minX: viewBox.x,
      minY: viewBox.y,
      width: Math.ceil(viewBox.width),
      height: Math.ceil(viewBox.height),
    };
  }

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
  const [initialState, setInitialState] = useState<PanZoomState>({ scale: 1, position: { x: 0, y: 0 } });
  const [viewportHeight, setViewportHeight] = useState(MIN_VIEWPORT_HEIGHT);

  const updateInitialView = useCallback(() => {
    const viewport = viewportRef.current;
    const styles = viewport ? window.getComputedStyle(viewport) : null;
    const paddingLeft = styles ? parseFloat(styles.paddingLeft || '0') : 0;
    const paddingRight = styles ? parseFloat(styles.paddingRight || '0') : 0;
    const paddingTop = styles ? parseFloat(styles.paddingTop || '0') : 0;
    const paddingBottom = styles ? parseFloat(styles.paddingBottom || '0') : 0;
    const paddingX = paddingLeft + paddingRight;
    const paddingY = paddingTop + paddingBottom;
    const availableWidth = Math.max(1, (viewport?.clientWidth || 600) - paddingX);

    const widthFitScale = baseSize.width > 0 ? availableWidth / baseSize.width : 1;
    const scale = Number.isFinite(widthFitScale) && widthFitScale > 0 ? widthFitScale : 1;
    const scaledWidth = baseSize.width * scale;
    const scaledHeight = baseSize.height * scale;
    const nextViewportHeight = Math.max(MIN_VIEWPORT_HEIGHT, Math.ceil(scaledHeight + paddingY));
    const availableHeight = Math.max(1, nextViewportHeight - paddingY);

    setViewportHeight(nextViewportHeight);
    setInitialState({
      scale,
      position: {
        x: Math.max(0, (availableWidth - scaledWidth) / 2),
        y: Math.max(0, (availableHeight - scaledHeight) / 2),
      },
    });
  }, [baseSize.width, baseSize.height]);

  useEffect(() => {
    if (!svg || !containerRef.current) return;
    const el = containerRef.current.querySelector<SVGSVGElement>('svg');
    if (!el) return;

    const nextBaseSize = readSvgSize(el);
    const originalViewBox = el.getAttribute('viewBox') || `${nextBaseSize.minX} ${nextBaseSize.minY} ${nextBaseSize.width} ${nextBaseSize.height}`;

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

  useEffect(() => {
    updateInitialView();
  }, [baseSize, updateInitialView]);

  useEffect(() => {
    const el = viewportRef.current;
    if (!el) return;
    const ro = new ResizeObserver(() => updateInitialView());
    ro.observe(el);
    return () => ro.disconnect();
  }, [updateInitialView]);

  const panZoom = usePanZoom(panZoomTarget, { initialState });

  useEffect(() => {
    const eventTarget = viewportRef.current;
    if (!eventTarget) return;
    eventTarget.addEventListener('wheel', panZoom.onWheel, { passive: false });
    eventTarget.addEventListener('pointerdown', panZoom.onPointerDown);
    eventTarget.addEventListener('pointermove', panZoom.onPointerMove);
    eventTarget.addEventListener('pointerup', panZoom.onPointerUp);
    eventTarget.addEventListener('pointercancel', panZoom.onPointerUp);
    eventTarget.addEventListener('lostpointercapture', panZoom.onPointerUp);
    eventTarget.addEventListener('dblclick', panZoom.zoomIn);
    return () => {
      eventTarget.removeEventListener('wheel', panZoom.onWheel);
      eventTarget.removeEventListener('pointerdown', panZoom.onPointerDown);
      eventTarget.removeEventListener('pointermove', panZoom.onPointerMove);
      eventTarget.removeEventListener('pointerup', panZoom.onPointerUp);
      eventTarget.removeEventListener('pointercancel', panZoom.onPointerUp);
      eventTarget.removeEventListener('lostpointercapture', panZoom.onPointerUp);
      eventTarget.removeEventListener('dblclick', panZoom.zoomIn);
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
        style={{ height: viewportHeight }}
      >
        <div
          className="mermaid-stage"
          style={{
            width: baseSize.width,
            height: baseSize.height,
          }}
        >
          <div className="mermaid" ref={containerRef}>
            {err ? (
              <div className="mermaid-error">
                <div className="mermaid-error-icon">️</div>
                <div className="mermaid-error-text">Render failed</div>
                <pre>{err}</pre>
              </div>
            ) : svg ? (
              <div dangerouslySetInnerHTML={{ __html: svg }} />
            ) : (
              <div className="mermaid-loading">
                <div className="mermaid-loading-spinner" />
                <span>Rendering diagram...</span>
              </div>
            )}
          </div>
        </div>
      </div>
    </section>
  );
}
