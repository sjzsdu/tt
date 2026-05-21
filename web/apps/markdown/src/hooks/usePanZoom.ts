import { useState, useCallback, useEffect, useRef } from 'react';

interface PanZoomState {
  scale: number;
  position: { x: number; y: number };
}

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

  const zoomIn = useCallback(() =>
    setState(s => {
      const el = targetRef.current;
      if (!el) return s;
      const rect = el.parentElement?.getBoundingClientRect();
      if (!rect) return s;
      const centerX = rect.width / 2;
      const centerY = rect.height / 2;
      const newScale = Math.min(maxScale, s.scale + step);
      const scaleRatio = newScale / s.scale;
      return {
        scale: newScale,
        position: {
          x: centerX - scaleRatio * (centerX - s.position.x),
          y: centerY - scaleRatio * (centerY - s.position.y),
        },
      };
    }), [maxScale, step]);
  const zoomOut = useCallback(() =>
    setState(s => {
      const el = targetRef.current;
      if (!el) return s;
      const rect = el.parentElement?.getBoundingClientRect();
      if (!rect) return s;
      const centerX = rect.width / 2;
      const centerY = rect.height / 2;
      const newScale = Math.max(minScale, s.scale - step);
      const scaleRatio = newScale / s.scale;
      return {
        scale: newScale,
        position: {
          x: centerX - scaleRatio * (centerX - s.position.x),
          y: centerY - scaleRatio * (centerY - s.position.y),
        },
      };
    }), [minScale, step]);
  const reset = useCallback(() => setState(initialState), [initialState]);

  const setScale = useCallback((scale: number) =>
    setState(s => ({ ...s, scale: Math.min(maxScale, Math.max(minScale, scale)) })), [maxScale, minScale]);

  const onWheel = useCallback((e: WheelEvent) => {
    e.preventDefault();
    const el = e.currentTarget as HTMLElement | null;
    if (!el) return;
    const rect = el.getBoundingClientRect();
    const mouseX = e.clientX - rect.left;
    const mouseY = e.clientY - rect.top;

    const factor = e.deltaY > 0 ? 0.95 : 1.05;
    setState(s => {
      const newScale = Math.min(maxScale, Math.max(minScale, s.scale * factor));
      const scaleRatio = newScale / s.scale;
      return {
        scale: newScale,
        position: {
          x: mouseX - scaleRatio * (mouseX - s.position.x),
          y: mouseY - scaleRatio * (mouseY - s.position.y),
        },
      };
    });
  }, [maxScale, minScale]);

  const onPointerDown = useCallback((e: PointerEvent) => {
    const el = e.currentTarget as HTMLElement | null;
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
    const el = e.currentTarget as HTMLElement | null;
    if (!el) return;
    if (el.hasPointerCapture(e.pointerId)) {
      el.releasePointerCapture(e.pointerId);
    }
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
