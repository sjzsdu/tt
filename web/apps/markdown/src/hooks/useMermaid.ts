import { useState, useEffect, useRef } from 'react';

type AppTheme = 'light' | 'dark';

function createMermaidConfig(theme: AppTheme) {
  const dark = theme === 'dark';
  const palette = dark ? {
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
    section0: '#111827',
    section0Bkg: '#111827',
    section1: '#172033',
    section1Bkg: '#172033',
    section2: '#1e293b',
    section2Bkg: '#1e293b',
    section3: '#0f172a',
    section3Bkg: '#0f172a',
    altSectionBkgColor: '#172033',
    altSectionBkgColor2: '#1e293b',
    cScale0: '#111827',
    cScale1: '#172033',
    cScale2: '#1e293b',
  } : {
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
    section0: '#ffffff',
    section0Bkg: '#ffffff',
    section1: '#f8fafc',
    section1Bkg: '#f8fafc',
    section2: '#eef4ff',
    section2Bkg: '#eef4ff',
    section3: '#f1f5f9',
    section3Bkg: '#f1f5f9',
    altSectionBkgColor: '#f8fafc',
    altSectionBkgColor2: '#eef4ff',
    cScale0: '#ffffff',
    cScale1: '#f8fafc',
    cScale2: '#eef4ff',
  };

  return {
    startOnLoad: false,
    theme: 'base' as const,
    securityLevel: 'loose' as const,
    logLevel: 'error' as const,
    flowchart: { useMaxWidth: true, htmlLabels: false, curve: 'basis' as const },
    themeVariables: palette,
    themeCSS: dark ? `
      .node rect, .node circle, .node ellipse, .node polygon, .node path { fill: #111827; stroke: #38bdf8; stroke-width: 1.5px; rx: 6px; ry: 6px; }
      .edgePath .path { stroke: #94a3b8; stroke-width: 1.5px; }
      .edgeLabel, .edgeLabel rect { background-color: #0f172a; fill: #0f172a; color: #cbd5e1; font-size: 12px; }
      .cluster rect { fill: #111827; stroke: #334155; stroke-width: 1px; }
      .actor, .actor-line { stroke: #64748b !important; }
      .actor > rect, .actor-man, .actor-box { fill: #172033 !important; stroke: #64748b !important; }
      .labelBox, .labelBox rect, .labelBox polygon, .loopLine, .loopLine > rect, .loopLine > polygon, .note, .note rect, .note polygon, .sequenceNumber { fill: #1e293b !important; stroke: #64748b !important; }
      .label, .label text, .label span, .nodeLabel, .nodeLabel p, .nodeLabel span, .edgeLabel, .edgeLabel text, .sectionTitle, .actor, .actor text, .messageText, .loopText, .noteText, .taskText, .taskTextOutsideRight, .taskTextOutsideLeft, .sequenceNumber text, .sequenceNumber, .labelBox text, .labelBox tspan, .loopLine text, .loopLine tspan, .note text, .note tspan, .mindmap-node .label, .mindmap-node .label text, .mindmap-node .label span, svg text, svg tspan { fill: #e5eefc !important; color: #e5eefc !important; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 13px; }
    ` : `
      .node rect, .node circle, .node ellipse, .node polygon, .node path { fill: #fff; stroke: #cbd5e1; stroke-width: 1.5px; rx: 6px; ry: 6px; }
      .edgePath .path { stroke: #94a3b8; stroke-width: 1.5px; }
      .edgeLabel, .edgeLabel rect { background-color: #fff; fill: #fff; color: #475569; font-size: 12px; }
      .cluster rect { fill: #f8fafc; stroke: #e2e8f0; stroke-width: 1px; }
      .actor, .actor-line { stroke: #cbd5e1 !important; }
      .actor > rect, .actor-man, .actor-box { fill: #ffffff !important; stroke: #cbd5e1 !important; }
      .labelBox, .labelBox rect, .labelBox polygon, .loopLine, .loopLine > rect, .loopLine > polygon, .note, .note rect, .note polygon, .sequenceNumber { fill: #eef4ff !important; stroke: #cbd5e1 !important; }
      .label, .label text, .label span, .nodeLabel, .nodeLabel p, .nodeLabel span, .edgeLabel, .edgeLabel text, .sectionTitle, .actor, .actor text, .messageText, .loopText, .noteText, .taskText, .taskTextOutsideRight, .taskTextOutsideLeft, .sequenceNumber text, .sequenceNumber, .labelBox text, .labelBox tspan, .loopLine text, .loopLine tspan, .note text, .note tspan, .mindmap-node .label, .mindmap-node .label text, .mindmap-node .label span, svg text, svg tspan { fill: #0f172a !important; color: #0f172a !important; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; font-size: 13px; }
    `,
  };
}

export function useMermaid(code: string, index: number, theme: AppTheme) {
  const [svg, setSvg] = useState('');
  const [err, setErr] = useState('');
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const cancelledRef = useRef(false);
  const idRef = useRef(`mermaid-${index}-${Date.now()}`);

  useEffect(() => {
    if (timerRef.current) clearTimeout(timerRef.current);
    cancelledRef.current = false;
    timerRef.current = setTimeout(() => {
      idRef.current = `mermaid-${index}-${Date.now()}-${Math.random().toString(16).slice(2)}`;
      void import('mermaid')
        .then(module => {
          const mermaid = module.default;
          mermaid.initialize(createMermaidConfig(theme));
          return mermaid.render(idRef.current, code);
        })
        .then(r => {
          if (!cancelledRef.current) {
            setSvg(r.svg);
            setErr('');
          }
        })
        .catch(e => {
          if (!cancelledRef.current) setErr(String(e));
        });
    }, 150);
    return () => {
      cancelledRef.current = true;
      if (timerRef.current) clearTimeout(timerRef.current);
    };
  }, [code, index, theme]);

  return { svg, err };
}
