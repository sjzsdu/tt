export function addPngExportStyle(svg: SVGSVGElement) {
  const style = document.createElementNS('http://www.w3.org/2000/svg', 'style');
  style.textContent = [
    'svg { background: #ffffff; }',
    'text, tspan { fill: #111827 !important; font-family: "Noto Sans", Arial, "Liberation Sans", sans-serif !important; }',
  ].join('\n');
  svg.insertBefore(style, svg.firstChild);
}
