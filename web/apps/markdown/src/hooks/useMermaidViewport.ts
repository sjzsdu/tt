import { useCallback, useEffect, useRef, useState } from 'react';

export interface SvgSize {
  minX: number;
  minY: number;
  width: number;
  height: number;
}

export interface PanZoomState {
  scale: number;
  position: { x: number; y: number };
}

const MIN_VIEWPORT_HEIGHT = 300;
const MAX_VIEWPORT_HEIGHT = 720;
const VIEWPORT_HEIGHT_RATIO = 0.72;

function clampViewportHeight(height: number) {
  const viewportCap = typeof window === 'undefined' ? MAX_VIEWPORT_HEIGHT : Math.floor(window.innerHeight * VIEWPORT_HEIGHT_RATIO);
  return Math.max(MIN_VIEWPORT_HEIGHT, Math.min(MAX_VIEWPORT_HEIGHT, viewportCap, height));
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

  return { minX, minY, width: maxX - minX, height: maxY - minY };
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
    return { minX: viewBox.x, minY: viewBox.y, width: Math.ceil(viewBox.width), height: Math.ceil(viewBox.height) };
  }

  const width = Number(svg.getAttribute('width')) || rect.width || 600;
  const height = Number(svg.getAttribute('height')) || rect.height || 300;
  return { minX: 0, minY: 0, width: Math.max(1, Math.ceil(width)), height: Math.max(1, Math.ceil(height)) };
}

function prepareSvgForViewport(svg: SVGSVGElement, size: SvgSize) {
  const originalViewBox = svg.getAttribute('viewBox') || `${size.minX} ${size.minY} ${size.width} ${size.height}`;
  svg.setAttribute('viewBox', originalViewBox);
  svg.setAttribute('width', String(size.width));
  svg.setAttribute('height', String(size.height));
  svg.dataset.originalViewBox = originalViewBox;
  svg.dataset.exportWidth = String(size.width);
  svg.dataset.exportHeight = String(size.height);
  svg.style.width = `${size.width}px`;
  svg.style.height = `${size.height}px`;
  svg.style.maxWidth = 'none';
  svg.style.display = 'block';
}

export function useMermaidViewport(svg: string) {
  const viewportRef = useRef<HTMLDivElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const svgElRef = useRef<SVGSVGElement | null>(null);
  const [panZoomTarget, setPanZoomTarget] = useState<HTMLDivElement | null>(null);
  const [baseSize, setBaseSize] = useState<SvgSize>({ minX: 0, minY: 0, width: 600, height: 300 });
  const [initialState, setInitialState] = useState<PanZoomState>({ scale: 1, position: { x: 0, y: 0 } });
  const [viewportHeight, setViewportHeight] = useState(MIN_VIEWPORT_HEIGHT);

  const updateInitialView = useCallback(() => {
    const viewport = viewportRef.current;
    const styles = viewport ? window.getComputedStyle(viewport) : null;
    const paddingX = (styles ? parseFloat(styles.paddingLeft || '0') + parseFloat(styles.paddingRight || '0') : 0);
    const paddingY = (styles ? parseFloat(styles.paddingTop || '0') + parseFloat(styles.paddingBottom || '0') : 0);
    const availableWidth = Math.max(1, (viewport?.clientWidth || 600) - paddingX);
    const nextViewportHeight = clampViewportHeight(Math.ceil(baseSize.height + paddingY));
    const availableHeight = Math.max(1, nextViewportHeight - paddingY);
    const fitScale = Math.min(
      baseSize.width > 0 ? availableWidth / baseSize.width : 1,
      baseSize.height > 0 ? availableHeight / baseSize.height : 1,
      1.25
    );
    const scale = Number.isFinite(fitScale) && fitScale > 0 ? fitScale : 1;

    setViewportHeight(nextViewportHeight);
    setInitialState({
      scale,
      position: {
        x: Math.max(0, (availableWidth - baseSize.width * scale) / 2),
        y: Math.max(0, (availableHeight - baseSize.height * scale) / 2),
      },
    });
  }, [baseSize.width, baseSize.height]);

  useEffect(() => {
    if (!svg || !containerRef.current) return;
    const el = containerRef.current.querySelector<SVGSVGElement>('svg');
    if (!el) return;
    const nextBaseSize = readSvgSize(el);
    svgElRef.current = el;
    prepareSvgForViewport(el, nextBaseSize);
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

  return { viewportRef, containerRef, svgElRef, panZoomTarget, baseSize, initialState, viewportHeight };
}
