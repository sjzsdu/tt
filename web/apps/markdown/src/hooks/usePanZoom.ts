import { useState, useCallback, useEffect, useRef } from 'react';

interface PanZoomState {
  scale: number;
  position: { x: number; y: number };
}

interface SvgViewBox {
  minX: number;
  minY: number;
  width: number;
  height: number;
}

interface UsePanZoomOptions {
  minScale?: number;
  maxScale?: number;
  step?: number;
}

const initialState: PanZoomState = { scale: 1, position: { x: 0, y: 0 } };

export function usePanZoom(
  svgEl: SVGSVGElement | null,
  baseViewBox: SvgViewBox,
  options: UsePanZoomOptions = {}
) {
  const { minScale = 0.4, maxScale = 4, step = 0.2 } = options;
  const svgRef = useRef(svgEl);
  svgRef.current = svgEl;
  const [state, setState] = useState<PanZoomState>(initialState);

  useEffect(() => {
    setState(initialState);
  }, [svgEl, baseViewBox.minX, baseViewBox.minY, baseViewBox.width, baseViewBox.height]);

  const zoomIn = useCallback(() =>
    setState(s => ({ ...s, scale: Math.min(maxScale, s.scale + step) })), [maxScale, step]);
  const zoomOut = useCallback(() =>
    setState(s => ({ ...s, scale: Math.max(minScale, s.scale - step) })), [minScale, step]);
  const reset = useCallback(() => setState(initialState), []);

  const setScale = useCallback((scale: number) =>
    setState(s => ({ ...s, scale: Math.min(maxScale, Math.max(minScale, scale)) })), [maxScale, minScale]);

  const onWheel = useCallback((e: WheelEvent) => {
    e.preventDefault();
    const factor = e.deltaY > 0 ? 0.95 : 1.05;
    setState(s => ({
      ...s,
      scale: Math.min(maxScale, Math.max(minScale, s.scale * factor)),
    }));
  }, [maxScale, minScale]);

  const onPointerDown = useCallback((e: PointerEvent) => {
    const el = svgRef.current;
    if (!el) return;
    el.style.cursor = 'grabbing';
    el.setPointerCapture(e.pointerId);
  }, []);

  const onPointerMove = useCallback((e: PointerEvent) => {
    if (e.buttons !== 1) return;
    setState(s => ({
      ...s,
      position: { x: s.position.x + e.movementX, y: s.position.y + e.movementY },
    }));
  }, []);

  const onPointerUp = useCallback((e: PointerEvent) => {
    const el = svgRef.current;
    if (!el) return;
    if (el.hasPointerCapture(e.pointerId)) {
      el.releasePointerCapture(e.pointerId);
    }
    el.style.cursor = 'grab';
  }, []);

  useEffect(() => {
    const el = svgRef.current;
    if (!el) return;

    const width = baseViewBox.width / state.scale;
    const height = baseViewBox.height / state.scale;
    const minX = baseViewBox.minX - state.position.x / state.scale;
    const minY = baseViewBox.minY - state.position.y / state.scale;
    el.setAttribute('viewBox', `${minX} ${minY} ${width} ${height}`);
    el.style.transform = '';
    el.style.transformOrigin = '';
  }, [baseViewBox, state]);

  return {
    scale: state.scale,
    position: state.position,
    zoomIn,
    zoomOut,
    reset,
    setScale,
    onWheel,
    onPointerDown,
    onPointerMove,
    onPointerUp,
  };
}
