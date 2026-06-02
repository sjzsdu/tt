import { type ExportGeometry, getExportGeometry, getMarkupGeometry, parsePositiveNumber } from './geometry';

export function inlineComputedTextStyles(source: SVGSVGElement, clone: SVGSVGElement) {
  const sourceText = source.querySelectorAll<SVGTextElement | SVGTSpanElement>('text,tspan');
  const cloneText = clone.querySelectorAll<SVGTextElement | SVGTSpanElement>('text,tspan');
  sourceText.forEach((node, index) => {
    const target = cloneText[index];
    if (!target) return;
    const style = getComputedStyle(node);
    target.style.fill = style.fill && style.fill !== 'none' ? style.fill : '#111827';
    target.style.fontFamily = style.fontFamily || 'Arial, sans-serif';
    target.style.fontSize = style.fontSize || target.getAttribute('font-size') || '16px';
    target.style.fontWeight = style.fontWeight || target.getAttribute('font-weight') || '400';
    target.style.fontStyle = style.fontStyle || 'normal';
    target.style.opacity = style.opacity || '1';
    target.setAttribute('fill', target.style.fill || '#111827');
    target.setAttribute('font-family', target.style.fontFamily || 'Arial, sans-serif');
    target.setAttribute('font-size', target.style.fontSize || '16px');
    target.setAttribute('font-weight', target.style.fontWeight || '400');
  });
}

export function inlineComputedSvgStyles(source: SVGSVGElement, clone: SVGSVGElement) {
  const sourceNodes = [source, ...Array.from(source.querySelectorAll<SVGElement>('*'))];
  const cloneNodes = [clone, ...Array.from(clone.querySelectorAll<SVGElement>('*'))];
  sourceNodes.forEach((node, index) => {
    const target = cloneNodes[index];
    if (!target) return;
    const style = getComputedStyle(node);
    target.style.fill = style.fill || target.style.fill;
    target.style.stroke = style.stroke || target.style.stroke;
    target.style.strokeWidth = style.strokeWidth || target.style.strokeWidth;
    target.style.strokeLinecap = style.strokeLinecap || target.style.strokeLinecap;
    target.style.strokeLinejoin = style.strokeLinejoin || target.style.strokeLinejoin;
    target.style.opacity = style.opacity || target.style.opacity;
    target.style.display = style.display || target.style.display;
    target.style.visibility = style.visibility || target.style.visibility;
  });
}

export function replaceForeignObjectsWithText(source: SVGSVGElement, clone: SVGSVGElement) {
  const sourceObjects = source.querySelectorAll<SVGForeignObjectElement>('foreignObject');
  const cloneObjects = clone.querySelectorAll<SVGForeignObjectElement>('foreignObject');
  sourceObjects.forEach((sourceObject, index) => {
    const cloneObject = cloneObjects[index];
    if (!cloneObject) return;
    const text = (sourceObject.textContent || '').replace(/\s+/g, ' ').trim();
    if (!text) {
      cloneObject.remove();
      return;
    }
    const x = parsePositiveNumber(sourceObject.getAttribute('x')) || 0;
    const y = parsePositiveNumber(sourceObject.getAttribute('y')) || 0;
    const width = parsePositiveNumber(sourceObject.getAttribute('width')) || 1;
    const height = parsePositiveNumber(sourceObject.getAttribute('height')) || 16;
    const style = getComputedStyle(sourceObject);
    const textEl = document.createElementNS('http://www.w3.org/2000/svg', 'text');
    textEl.textContent = text;
    textEl.setAttribute('x', String(x + width / 2));
    textEl.setAttribute('y', String(y + height / 2));
    textEl.setAttribute('text-anchor', 'middle');
    textEl.setAttribute('dominant-baseline', 'middle');
    textEl.setAttribute('fill', style.color && style.color !== 'rgba(0, 0, 0, 0)' ? style.color : '#111827');
    textEl.setAttribute('font-family', style.fontFamily || 'Arial, sans-serif');
    textEl.setAttribute('font-size', style.fontSize || '16px');
    textEl.setAttribute('font-weight', style.fontWeight || '400');
    const transform = cloneObject.getAttribute('transform') || sourceObject.getAttribute('transform');
    if (transform) textEl.setAttribute('transform', transform);
    cloneObject.replaceWith(textEl);
  });
}

export function forceExportTextStyles(svg: SVGSVGElement) {
  svg.querySelectorAll<SVGTextElement | SVGTSpanElement>('text,tspan').forEach(node => {
    const fill = node.getAttribute('fill') || node.style.fill;
    if (!fill || fill === 'none' || fill === 'transparent') {
      node.setAttribute('fill', '#111827');
      node.style.fill = '#111827';
    }
    if (!node.getAttribute('font-family') && !node.style.fontFamily) {
      node.setAttribute('font-family', 'Arial, Helvetica, sans-serif');
      node.style.fontFamily = 'Arial, Helvetica, sans-serif';
    }
    if (!node.getAttribute('font-size') && !node.style.fontSize) {
      node.setAttribute('font-size', '16px');
      node.style.fontSize = '16px';
    }
    if (!node.getAttribute('opacity') && !node.style.opacity) {
      node.setAttribute('opacity', '1');
      node.style.opacity = '1';
    }
  });
}

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
