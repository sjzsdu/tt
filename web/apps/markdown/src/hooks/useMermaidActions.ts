import type { RefObject } from 'react';

export function useMermaidActions(svg: string, svgElRef: RefObject<SVGSVGElement | null>) {
  const exportSvg = async () => {
    if (!svgElRef.current) return;
    const { svgToBlob, downloadBlob } = await import('../utils/export');
    downloadBlob(svgToBlob(svgElRef.current), 'mermaid-diagram.svg');
  };

  const exportPng = async () => {
    if (!svg) return;
    const { svgMarkupToPngBlob, downloadBlob } = await import('../utils/export');
    downloadBlob(await svgMarkupToPngBlob(svg), 'mermaid-diagram.png');
  };

  const copyPng = async () => {
    if (!svg) return;
    const { svgMarkupToPngBlob } = await import('../utils/export');
    const blob = await svgMarkupToPngBlob(svg);
    await navigator.clipboard.write([new ClipboardItem({ 'image/png': blob })]);
  };

  return { exportSvg, exportPng, copyPng };
}
