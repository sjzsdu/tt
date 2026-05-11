import { Canvg } from 'canvg';

export function svgToBlob(svg: SVGSVGElement): Blob {
  const clone = svg.cloneNode(true) as SVGSVGElement;
  clone.setAttribute('xmlns', 'http://www.w3.org/2000/svg');
  return new Blob([new XMLSerializer().serializeToString(clone)], {
    type: 'image/svg+xml;charset=utf-8',
  });
}

export async function svgToPngBlob(svg: SVGSVGElement): Promise<Blob> {
  const rect = svg.getBoundingClientRect();
  const clone = svg.cloneNode(true) as SVGSVGElement;
  clone.setAttribute('xmlns', 'http://www.w3.org/2000/svg');

  const styleEl = clone.querySelector('style');
  if (styleEl) styleEl.remove();

  const fontStyle = document.createElementNS('http://www.w3.org/2000/svg', 'style');
  fontStyle.textContent = [
    'text, tspan { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Arial, sans-serif !important; }',
    '.label, .nodeLabel, .edgeLabel { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Arial, sans-serif !important; }',
  ].join('\n');
  clone.insertBefore(fontStyle, clone.firstChild);

  let width = Math.max(
    1,
    Math.ceil(Number(clone.getAttribute('width')) || rect.width)
  );
  let height = Math.max(
    1,
    Math.ceil(Number(clone.getAttribute('height')) || rect.height)
  );
  clone.setAttribute('width', String(width));
  clone.setAttribute('height', String(height));

  const canvas = document.createElement('canvas');
  canvas.width = width * 2;
  canvas.height = height * 2;
  const ctx = canvas.getContext('2d')!;
  ctx.scale(2, 2);
  ctx.fillStyle = '#ffffff';
  ctx.fillRect(0, 0, width, height);

  const v = await Canvg.from(ctx, new XMLSerializer().serializeToString(clone), {
    DOMParser,
    ignoreMouse: true,
    ignoreAnimation: true,
  });
  await v.render();

  return new Promise<Blob>((resolve, reject) =>
    canvas.toBlob(b =>
      b ? resolve(b) : reject(new Error('PNG export failed')),
      'image/png'
    )
  );
}

export function downloadBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
