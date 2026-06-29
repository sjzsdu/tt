export type AppTheme = 'light' | 'dark';

const darkPalette = {
  background: '#0f172a',
  primaryColor: '#111827',
  primaryBorderColor: '#38bdf8',
  primaryTextColor: '#e5eefc',
  secondaryColor: '#172033',
  secondaryBorderColor: '#64748b',
  secondaryTextColor: '#e5eefc',
  tertiaryColor: '#1e293b',
  tertiaryBorderColor: '#334155',
  tertiaryTextColor: '#e5eefc',
  lineColor: '#94a3b8',
  textColor: '#e5eefc',
  mainBkg: '#111827',
  nodeBorder: '#38bdf8',
  clusterBkg: '#111827',
  clusterBorder: '#334155',
  actorBkg: '#172033',
  actorBorder: '#64748b',
  actorTextColor: '#e5eefc',
  labelBoxBkgColor: '#0f172a',
  labelBoxBorderColor: '#334155',
  labelTextColor: '#e5eefc',
  loopTextColor: '#e5eefc',
  noteBkgColor: '#1e293b',
  noteBorderColor: '#64748b',
  noteTextColor: '#e5eefc',
  sequenceNumberColor: '#e5eefc',
  activationBkgColor: '#334155',
  activationBorderColor: '#64748b',
  signalColor: '#94a3b8',
  signalTextColor: '#e5eefc',
  sectionBkgColor: '#111827',
  sectionBkgColor2: '#172033',
  sectionBkgColor3: '#1e293b',
  sectionBkgColor4: '#0f172a',
  sectionBkgColor5: '#111827',
};

const lightPalette = {
  background: '#ffffff',
  primaryColor: '#ffffff',
  primaryBorderColor: '#cbd5e1',
  primaryTextColor: '#0f172a',
  secondaryColor: '#f8fafc',
  secondaryBorderColor: '#cbd5e1',
  secondaryTextColor: '#0f172a',
  tertiaryColor: '#eef4ff',
  tertiaryBorderColor: '#cbd5e1',
  tertiaryTextColor: '#0f172a',
  lineColor: '#94a3b8',
  textColor: '#0f172a',
  mainBkg: '#ffffff',
  nodeBorder: '#cbd5e1',
  clusterBkg: '#f8fafc',
  clusterBorder: '#e2e8f0',
  actorBkg: '#ffffff',
  actorBorder: '#cbd5e1',
  actorTextColor: '#0f172a',
  labelBoxBkgColor: '#ffffff',
  labelBoxBorderColor: '#cbd5e1',
  labelTextColor: '#0f172a',
  loopTextColor: '#0f172a',
  noteBkgColor: '#eef4ff',
  noteBorderColor: '#cbd5e1',
  noteTextColor: '#0f172a',
  sequenceNumberColor: '#0f172a',
  activationBkgColor: '#e2e8f0',
  activationBorderColor: '#cbd5e1',
  signalColor: '#64748b',
  signalTextColor: '#0f172a',
  sectionBkgColor: '#ffffff',
  sectionBkgColor2: '#f8fafc',
  sectionBkgColor3: '#eef4ff',
  sectionBkgColor4: '#f1f5f9',
  sectionBkgColor5: '#ffffff',
};

const darkThemeCss = `
  .node rect, .node circle, .node ellipse, .node polygon, .node path { fill: #111827; stroke: #38bdf8; stroke-width: 1.5px; rx: 6px; ry: 6px; }
  .edgePath .path { stroke: #94a3b8; stroke-width: 1.5px; }
  .edgeLabel, .edgeLabel rect { background-color: #0f172a; fill: #0f172a; color: #cbd5e1; font-size: 12px; }
  .cluster rect { fill: #111827; stroke: #334155; stroke-width: 1px; }
  .actor-line { stroke: #64748b !important; }
  rect.actor, rect.actor-top, rect.actor-bottom, .actor > rect, rect.actor-box { fill: #172033 !important; stroke: #64748b !important; }
  text.actor, text.actor-box, text.actor tspan, text.actor-box tspan, .actor text, .actor tspan { fill: #e5eefc !important; color: #e5eefc !important; stroke: none !important; }
  .labelBox, .labelBox rect, .labelBox polygon, .note, .note rect, .note polygon, .sequenceNumber { fill: #1e293b !important; stroke: #64748b !important; }
  .label, .label text, .nodeLabel, .nodeLabel p, .edgeLabel, .edgeLabel text, .sectionTitle, text.actor, text.actor tspan, text.actor-box, text.actor-box tspan, .actor text, .actor tspan, .messageText, .loopText, .noteText, .taskText, .sequenceNumber text, .sequenceNumber, .labelBox text, .labelBox tspan, .note text, .note tspan, svg text, svg tspan { fill: #e5eefc !important; color: #e5eefc !important; stroke: none !important; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 13px; }
`;

const lightThemeCss = `
  .node rect, .node circle, .node ellipse, .node polygon, .node path { fill: #fff; stroke: #cbd5e1; stroke-width: 1.5px; rx: 6px; ry: 6px; }
  .edgePath .path { stroke: #94a3b8; stroke-width: 1.5px; }
  .edgeLabel, .edgeLabel rect { background-color: #fff; fill: #fff; color: #475569; font-size: 12px; }
  .cluster rect { fill: #f8fafc; stroke: #e2e8f0; stroke-width: 1px; }
  .actor-line { stroke: #cbd5e1 !important; }
  rect.actor, rect.actor-top, rect.actor-bottom, .actor > rect, rect.actor-box { fill: #ffffff !important; stroke: #cbd5e1 !important; }
  text.actor, text.actor-box, text.actor tspan, text.actor-box tspan, .actor text, .actor tspan { fill: #0f172a !important; color: #0f172a !important; stroke: none !important; }
  .labelBox, .labelBox rect, .labelBox polygon, .note, .note rect, .note polygon, .sequenceNumber { fill: #eef4ff !important; stroke: #cbd5e1 !important; }
  .label, .label text, .nodeLabel, .nodeLabel p, .edgeLabel, .edgeLabel text, .sectionTitle, text.actor, text.actor tspan, text.actor-box, text.actor-box tspan, .actor text, .actor tspan, .messageText, .loopText, .noteText, .taskText, .sequenceNumber text, .sequenceNumber, .labelBox text, .labelBox tspan, .note text, .note tspan, svg text, svg tspan { fill: #0f172a !important; color: #0f172a !important; stroke: none !important; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 13px; }
`;

export function mermaidThemeVariables(theme: AppTheme) {
  return theme === 'dark' ? darkPalette : lightPalette;
}

export function mermaidThemeCss(theme: AppTheme) {
  return theme === 'dark' ? darkThemeCss : lightThemeCss;
}

export function createMermaidConfig(theme: AppTheme) {
  return {
    startOnLoad: false,
    suppressErrorRendering: true,
    theme: 'base' as const,
    securityLevel: 'loose' as const,
    logLevel: 'error' as const,
    flowchart: { useMaxWidth: true, htmlLabels: true, curve: 'basis' as const },
    themeVariables: mermaidThemeVariables(theme),
    themeCSS: mermaidThemeCss(theme),
  };
}
