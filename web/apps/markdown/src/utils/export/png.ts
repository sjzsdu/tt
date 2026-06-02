import { renderPngWithBrowser } from './browser';
import { addPngExportStyle } from './pngStyle';
import { ensureResvgReady, renderPngWithResvg } from './resvg';
import { cloneForExport, forceExportTextStyles, prepareMarkupSvgForPng, serializedSvg } from './svg';

export async function svgToPngBlob(svg: SVGSVGElement): Promise<Blob> {
  await ensureResvgReady();
  const { clone, geometry } = cloneForExport(svg, 96);
  forceExportTextStyles(clone);
  addPngExportStyle(clone);
  return renderPngWithFallback(serializedSvg(clone), geometry);
}

export async function svgMarkupToPngBlob(svgMarkup: string): Promise<Blob> {
  await ensureResvgReady();
  const { clone, geometry } = prepareMarkupSvgForPng(svgMarkup);
  addPngExportStyle(clone);
  return renderPngWithFallback(serializedSvg(clone), geometry);
}

async function renderPngWithFallback(svgText: string, geometry: Parameters<typeof renderPngWithResvg>[1]) {
  try {
    return await renderPngWithBrowser(svgText, geometry);
  } catch {
    return renderPngWithResvg(svgText, geometry);
  }
}
