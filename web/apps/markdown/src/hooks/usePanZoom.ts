import { useState, useCallback, useEffect, useRef } from 'react';
import {
  clampScale,
  elementCenter,
  eventPointInElement,
  panBy,
  zoomAroundPoint,
  type PanZoomState,
} from '../utils/panZoomMath';

export type { PanZoomState } from '../utils/panZoomMath';

interface UsePanZoomOptions {
  minScale?: number;
  maxScale?: number;
  step?: number;
}

const defaultInitialState: PanZoomState = { scale: 1, position: { x: 0, y: 0 } };

export function usePanZoom(
  targetEl: HTMLElement | null,
  options: UsePanZoomOptions & { initialState?: PanZoomState } = {}
) {
  const { minScale = 0.4, maxScale = 4, step = 0.2, initialState = defaultInitialState } = options;
  const targetRef = useRef(targetEl);
  targetRef.current = targetEl;
  const [state, setState] = useState<PanZoomState>(initialState);

  useEffect(() => {
    setState(initialState);
  }, [targetEl]);

  useEffect(() => {
    if (targetEl) setState(initialState);
  }, [initialState]);

  const zoomBy = useCallback((delta: number, limit: number) => {
    setState(current => {
      const el = targetRef.current;
      if (!el) return current;
      const focus = elementCenter(el);
      if (!focus) return current;
      const nextScale = delta > 0 ? Math.min(limit, current.scale + delta) : Math.max(limit, current.scale + delta);
      return zoomAroundPoint(current, nextScale, focus);
    });
  }, []);

  const zoomIn = useCallback(() => zoomBy(step, maxScale), [maxScale, step, zoomBy]);
  const zoomOut = useCallback(() => zoomBy(-step, minScale), [minScale, step, zoomBy]);
  const reset = useCallback(() => setState(initialState), [initialState]);

  const setScale = useCallback((scale: number) =>
    setState(current => ({ ...current, scale: clampScale(scale, minScale, maxScale) })), [maxScale, minScale]);

  const onWheel = useCallback((event: WheelEvent) => {
    event.preventDefault();
    const el = event.currentTarget as HTMLElement | null;
    if (!el) return;
    const focus = eventPointInElement(event, el);
    const factor = event.deltaY > 0 ? 0.95 : 1.05;
    setState(current => zoomAroundPoint(current, clampScale(current.scale * factor, minScale, maxScale), focus));
  }, [maxScale, minScale]);

  const onPointerDown = useCallback((event: PointerEvent) => {
    const el = event.currentTarget as HTMLElement | null;
    if (!el) return;
    el.style.cursor = 'grabbing';
    el.setPointerCapture(event.pointerId);
  }, []);

  const onPointerMove = useCallback((event: PointerEvent) => {
    if (event.buttons !== 1) return;
    setState(current => panBy(current, { x: event.movementX, y: event.movementY }));
  }, []);

  const onPointerUp = useCallback((event: PointerEvent) => {
    const el = event.currentTarget as HTMLElement | null;
    if (!el) return;
    if (el.hasPointerCapture(event.pointerId)) el.releasePointerCapture(event.pointerId);
    el.style.cursor = 'grab';
  }, []);

  useEffect(() => {
    const el = targetRef.current;
    if (!el) return;
    el.style.transform = `translate(${state.position.x}px, ${state.position.y}px) scale(${state.scale})`;
    el.style.transformOrigin = '0 0';
    el.style.cursor = 'grab';
  }, [state]);

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

export type PanZoomApi = ReturnType<typeof usePanZoom>;
