import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { prepareSvgMarkupForViewport, type SvgSize } from '../utils/svgViewport';
import type { PanZoomState } from './usePanZoom';

const MIN_VIEWPORT_HEIGHT = 320;
const MAX_VIEWPORT_HEIGHT = 720;
const VIEWPORT_HEIGHT_RATIO = 0.72;
const WIDTH_TO_HEIGHT_RATIO = 0.62;
const FIT_SCALE_GUTTER = 0.94;
const MIN_INITIAL_SCALE = 0.08;
const MAX_INITIAL_SCALE = 1.15;
const DEFAULT_BASE_SIZE: SvgSize = { minX: 0, minY: 0, width: 640, height: 360 };

function clampViewportHeight(height: number) {
  const viewportCap = typeof window === 'undefined' ? MAX_VIEWPORT_HEIGHT : Math.floor(window.innerHeight * VIEWPORT_HEIGHT_RATIO);
  return Math.max(MIN_VIEWPORT_HEIGHT, Math.min(MAX_VIEWPORT_HEIGHT, viewportCap, height));
}

function preferredViewportHeight(baseHeight: number, paddingY: number, viewportWidth: number) {
  const naturalHeight = Math.ceil(baseHeight + paddingY);
  const widthBasedHeight = Math.ceil(viewportWidth * WIDTH_TO_HEIGHT_RATIO);
  return clampViewportHeight(Math.min(naturalHeight, Math.max(MIN_VIEWPORT_HEIGHT, widthBasedHeight)));
}

function nearlyEqual(a: number, b: number) {
  return Math.abs(a - b) < 0.5;
}

export function useD2Viewport(svg: string) {
  const viewportRef = useRef<HTMLDivElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const svgElRef = useRef<SVGSVGElement | null>(null);
  const prepared = useMemo(() => prepareSvgMarkupForViewport(svg), [svg]);
  const displaySvg = prepared?.svg ?? svg;
  const baseSize = prepared?.size ?? DEFAULT_BASE_SIZE;
  const [panZoomTarget, setPanZoomTarget] = useState<HTMLDivElement | null>(null);
  const [initialState, setInitialState] = useState<PanZoomState>({ scale: 1, position: { x: 0, y: 0 } });
  const [viewportHeight, setViewportHeight] = useState(MIN_VIEWPORT_HEIGHT);

  const updateInitialView = useCallback(() => {
    const viewport = viewportRef.current;
    const styles = viewport ? window.getComputedStyle(viewport) : null;
    const paddingX = styles ? parseFloat(styles.paddingLeft || '0') + parseFloat(styles.paddingRight || '0') : 0;
    const paddingY = styles ? parseFloat(styles.paddingTop || '0') + parseFloat(styles.paddingBottom || '0') : 0;
    const viewportWidth = viewport?.clientWidth || 640;
    const availableWidth = Math.max(1, viewportWidth - paddingX);
    const nextViewportHeight = preferredViewportHeight(baseSize.height, paddingY, viewportWidth);
    const availableHeight = Math.max(1, nextViewportHeight - paddingY);
    const fitScale = Math.min(
      baseSize.width > 0 ? availableWidth / baseSize.width : 1,
      baseSize.height > 0 ? availableHeight / baseSize.height : 1,
      MAX_INITIAL_SCALE
    );
    const scale = Number.isFinite(fitScale) && fitScale > 0
      ? Math.max(MIN_INITIAL_SCALE, Math.min(MAX_INITIAL_SCALE, fitScale * FIT_SCALE_GUTTER))
      : 1;
    const nextInitialState = {
      scale,
      position: {
        x: Math.max(0, (availableWidth - baseSize.width * scale) / 2),
        y: Math.max(0, (availableHeight - baseSize.height * scale) / 2),
      },
    };

    setViewportHeight(current => (nearlyEqual(current, nextViewportHeight) ? current : nextViewportHeight));
    setInitialState(current => {
      if (
        nearlyEqual(current.scale, nextInitialState.scale) &&
        nearlyEqual(current.position.x, nextInitialState.position.x) &&
        nearlyEqual(current.position.y, nextInitialState.position.y)
      ) {
        return current;
      }
      return nextInitialState;
    });
  }, [baseSize.width, baseSize.height]);

  useLayoutEffect(() => {
    if (!displaySvg || !containerRef.current) {
      svgElRef.current = null;
      setPanZoomTarget(null);
      return;
    }
    const el = containerRef.current.querySelector<SVGSVGElement>('svg');
    if (!el) return;
    svgElRef.current = el;
    setPanZoomTarget(containerRef.current);
  }, [displaySvg]);

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

  return { viewportRef, containerRef, svgElRef, panZoomTarget, displaySvg, baseSize, initialState, viewportHeight };
}
