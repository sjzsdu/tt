import { useCallback, useEffect, useRef, useState } from 'react';
import { prepareSvgForViewport, readSvgSize, type SvgSize } from '../utils/svgViewport';

export type { SvgSize } from '../utils/svgViewport';

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
