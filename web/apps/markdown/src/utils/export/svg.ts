import { type ExportGeometry, getExportGeometry, getMarkupGeometry } from './geometry';
import {
  forceExportTextStyles,
  inlineComputedSvgStyles,
  inlineComputedTextStyles,
  replaceForeignObjectsWithText,
} from './svgStyles';

export { forceExportTextStyles } from './svgStyles';

export function normalizeSvgForExport(svg: SVGSVGElement, geometry: ExportGeometry) {
  svg.setAttribute('xmlns', 'http://www.w3.org/2000/svg');
  svg.setAttribute('viewBox', geometry.viewBox);
  svg.setAttribute('width', String(geometry.width));
  svg.setAttribute('height', String(geometry.height));
  svg.style.width = `${geometry.width}px`;
  svg.style.height = `${geometry.height}px`;
  svg.style.maxWidth = 'none';
  svg.style.transform = '';
  svg.style.transformOrigin = '';
  svg.style.cursor = '';
  svg.removeAttribute('data-original-view-box');
  svg.removeAttribute('data-export-width');
  svg.removeAttribute('data-export-height');
}

export function cloneForExport(svg: SVGSVGElement, padding = 0): { clone: SVGSVGElement; geometry: ExportGeometry } {
  const geometry = getExportGeometry(svg, padding);
  const clone = svg.cloneNode(true) as SVGSVGElement;
  normalizeSvgForExport(clone, geometry);
  inlineComputedSvgStyles(svg, clone);
  inlineComputedTextStyles(svg, clone);
  replaceForeignObjectsWithText(svg, clone);
  return { clone, geometry };
}

export function parseSvgMarkup(svgMarkup: string): SVGSVGElement {
  const doc = new DOMParser().parseFromString(svgMarkup, 'image/svg+xml');
  const parseError = doc.querySelector('parsererror');
  if (parseError) throw new Error(`SVG parse failed: ${parseError.textContent || 'unknown parser error'}`);
  const svg = doc.querySelector('svg');
  if (!svg) throw new Error('SVG parse failed: missing <svg> root');
  return svg as unknown as SVGSVGElement;
}

export function prepareMarkupSvgForPng(svgMarkup: string): { clone: SVGSVGElement; geometry: ExportGeometry } {
  const clone = parseSvgMarkup(svgMarkup);
  const geometry = getMarkupGeometry(clone, 96);
  normalizeSvgForExport(clone, geometry);
  replaceForeignObjectsWithText(clone, clone);
  forceExportTextStyles(clone);
  return { clone, geometry };
}

export function serializedSvg(svg: SVGSVGElement): string {
  const text = new XMLSerializer().serializeToString(svg);
  return text.startsWith('<?xml') ? text : `<?xml version="1.0" encoding="UTF-8"?>\n${text}`;
}

export function svgToBlob(svg: SVGSVGElement): Blob {
  const { clone } = cloneForExport(svg);
  return new Blob([serializedSvg(clone)], { type: 'image/svg+xml;charset=utf-8' });
}
