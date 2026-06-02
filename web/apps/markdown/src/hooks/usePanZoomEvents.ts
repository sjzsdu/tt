import { useEffect, type RefObject } from 'react';
import type { PanZoomApi } from './usePanZoom';

export function usePanZoomEvents(targetRef: RefObject<HTMLElement | null>, panZoom: PanZoomApi) {
  useEffect(() => {
    const target = targetRef.current;
    if (!target) return;
    target.addEventListener('wheel', panZoom.onWheel, { passive: false });
    target.addEventListener('pointerdown', panZoom.onPointerDown);
    target.addEventListener('pointermove', panZoom.onPointerMove);
    target.addEventListener('pointerup', panZoom.onPointerUp);
    target.addEventListener('pointercancel', panZoom.onPointerUp);
    target.addEventListener('lostpointercapture', panZoom.onPointerUp);
    target.addEventListener('dblclick', panZoom.zoomIn);
    return () => {
      target.removeEventListener('wheel', panZoom.onWheel);
      target.removeEventListener('pointerdown', panZoom.onPointerDown);
      target.removeEventListener('pointermove', panZoom.onPointerMove);
      target.removeEventListener('pointerup', panZoom.onPointerUp);
      target.removeEventListener('pointercancel', panZoom.onPointerUp);
      target.removeEventListener('lostpointercapture', panZoom.onPointerUp);
      target.removeEventListener('dblclick', panZoom.zoomIn);
    };
  }, [targetRef, panZoom]);
}
