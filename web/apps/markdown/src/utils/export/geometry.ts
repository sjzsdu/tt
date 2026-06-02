export interface ExportGeometry {
  viewBox: string;
  width: number;
  height: number;
}

export function parsePositiveNumber(value: string | null): number | null {
  if (!value) return null;
  if (value.trim().endsWith('%')) return null;
  const match = value.trim().match(/^([0-9]*\.?[0-9]+)/);
  if (!match) return null;
  const number = Number(match[1]);
  return Number.isFinite(number) && number > 0 ? number : null;
}

export function paddedGeometry(geometry: ExportGeometry, padding: number): ExportGeometry {
  if (padding <= 0) return geometry;
  const parts = geometry.viewBox.trim().split(/[\s,]+/).map(Number);
  if (parts.length !== 4 || !parts.every(Number.isFinite)) return geometry;
  return {
    viewBox: `${parts[0] - padding} ${parts[1] - padding} ${parts[2] + padding * 2} ${parts[3] + padding * 2}`,
    width: geometry.width + padding * 2,
    height: geometry.height + padding * 2,
  };
}

export function getExportGeometry(svg: SVGSVGElement, padding = 0): ExportGeometry {
  const dataWidth = parsePositiveNumber(svg.dataset.exportWidth ?? null);
  const dataHeight = parsePositiveNumber(svg.dataset.exportHeight ?? null);
  const originalViewBox = svg.dataset.originalViewBox || svg.getAttribute('viewBox') || '';
  const parts = originalViewBox.trim().split(/[\s,]+/).map(Number);

  if (parts.length === 4 && parts.every(Number.isFinite) && parts[2] > 0 && parts[3] > 0) {
    return paddedGeometry({
      viewBox: parts.join(' '),
      width: Math.ceil(dataWidth || parts[2]),
      height: Math.ceil(dataHeight || parts[3]),
    }, padding);
  }

  const rect = svg.getBoundingClientRect();
  const width = Math.ceil(dataWidth || parsePositiveNumber(svg.getAttribute('width')) || rect.width || 600);
  const height = Math.ceil(dataHeight || parsePositiveNumber(svg.getAttribute('height')) || rect.height || 300);
  return paddedGeometry({ viewBox: `0 0 ${width} ${height}`, width: Math.max(1, width), height: Math.max(1, height) }, padding);
}

export function getMarkupGeometry(svg: SVGSVGElement, padding = 0): ExportGeometry {
  const viewBox = svg.getAttribute('viewBox') || '';
  const parts = viewBox.trim().split(/[\s,]+/).map(Number);
  if (parts.length === 4 && parts.every(Number.isFinite) && parts[2] > 0 && parts[3] > 0) {
    return paddedGeometry({
      viewBox: parts.join(' '),
      width: Math.ceil(parsePositiveNumber(svg.getAttribute('width')) || parts[2]),
      height: Math.ceil(parsePositiveNumber(svg.getAttribute('height')) || parts[3]),
    }, padding);
  }

  const width = Math.ceil(parsePositiveNumber(svg.getAttribute('width')) || 600);
  const height = Math.ceil(parsePositiveNumber(svg.getAttribute('height')) || 300);
  return paddedGeometry({ viewBox: `0 0 ${width} ${height}`, width, height }, padding);
}
