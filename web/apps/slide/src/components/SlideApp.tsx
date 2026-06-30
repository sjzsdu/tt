import { useEffect, useRef, useState, useCallback } from 'react';
import Reveal from 'reveal.js';
import { parseSlides } from '../parser';
import { DEFAULT_TEMPLATE, getTemplate } from '../templates';
import type { SlideMeta } from '../types';
import { fetchSlideContent, fetchRawContent, fetchSlideList, createWS, type SlideFile } from '../api';
import { SlideContent } from './SlideContent';

const slidePositionKey = (file: string) => `tt-slide-position:${file}`;

interface SlideAppProps {
  contentMode: boolean;
  filePath?: string;
  templateOverride?: string;
}

export function SlideApp({ contentMode, filePath, templateOverride = '' }: SlideAppProps) {
  const [slides, setSlides] = useState<Awaited<ReturnType<typeof parseSlides>>['slides']>([]);
  const [meta, setMeta] = useState<SlideMeta | null>(null);
  const [error, setError] = useState('');
  const [isFullscreen, setIsFullscreen] = useState(false);
  const [files, setFiles] = useState<SlideFile[]>([]);
  const [currentFile, setCurrentFile] = useState(filePath || '');
  const [showFileList, setShowFileList] = useState(false);
  const [isListLoaded, setIsListLoaded] = useState(contentMode);
  const [deckVersion, setDeckVersion] = useState(0);
  const deckRef = useRef<Reveal.Api | null>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const wrapperRef = useRef<HTMLDivElement>(null);

  const loadSlides = useCallback(async (file?: string) => {
    try {
      let md: string;
      if (contentMode) {
        md = await fetchRawContent();
      } else if (file) {
        md = await fetchSlideContent(file);
      } else {
        setError('No slide file specified');
        return;
      }

      const { slides: parsed, meta: parsedMeta } = parseSlides(md);
      if (parsed.length === 0) {
        setError('No slides found. Use --- to separate slides.');
        return;
      }

      setMeta(parsedMeta);
      setSlides(parsed);
      setError('');
    } catch (e: any) {
      setError(String(e));
    }
  }, [contentMode]);

  useEffect(() => {
    fetchSlideList()
      .then(res => {
        setFiles(res.files);
        setIsListLoaded(true);
      })
      .catch(() => setIsListLoaded(true));
  }, []);

  useEffect(() => {
    if (currentFile) {
      loadSlides(currentFile);
    } else if (contentMode) {
      loadSlides();
    }
  }, [currentFile, contentMode, loadSlides]);

  useEffect(() => {
    if (slides.length === 0 || !containerRef.current) return;

    if (deckRef.current) {
      deckRef.current.destroy();
      deckRef.current = null;
    }

    const tpl = getTemplate(templateOverride || meta?.template || DEFAULT_TEMPLATE);

    const deck = new Reveal(containerRef.current, {
      hash: true,
      slideNumber: true,
      controls: true,
      progress: true,
      center: tpl.defaults.center,
      transition: meta?.transition || tpl.defaults.transition,
      width: tpl.defaults.width ?? 1200,
      height: tpl.defaults.height ?? 700,
      margin: tpl.defaults.margin ?? 0.04,
      minScale: 0.2,
      maxScale: 2.0,
    });

    deck.initialize().then(() => {
      deckRef.current = deck;
      setDeckVersion(v => v + 1);
    });

    return () => {
      deck.destroy();
      deckRef.current = null;
    };
  }, [slides, meta, templateOverride]);

  useEffect(() => {
    if (contentMode) return;
    const ws = createWS((data) => {
      if (data.type === 'reload') {
        loadSlides(currentFile);
      }
    });
    return () => ws.close();
  }, [contentMode, currentFile, loadSlides]);

  useEffect(() => {
    document.title = meta?.title || 'tt slide';
  }, [meta]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) return;
      if (e.key === 'f' || e.key === 'F') {
        e.preventDefault();
        toggleFullscreen();
      }
      if (e.key === 'Escape' && isFullscreen) {
        exitFullscreen();
      }
      if (e.key === 'l' || e.key === 'L') {
        if (files.length > 0) {
          e.preventDefault();
          setShowFileList(v => !v);
        }
      }
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [isFullscreen, files]);

  const toggleFullscreen = useCallback(() => {
    if (isFullscreen) {
      exitFullscreen();
    } else {
      enterFullscreen();
    }
  }, [isFullscreen]);

  const enterFullscreen = useCallback(() => {
    const el = wrapperRef.current;
    if (!el) return;
    if (el.requestFullscreen) {
      el.requestFullscreen();
    } else if ((el as any).webkitRequestFullscreen) {
      (el as any).webkitRequestFullscreen();
    }
  }, []);

  const exitFullscreen = useCallback(() => {
    if (document.fullscreenElement) {
      document.exitFullscreen();
    } else if ((document as any).webkitFullscreenElement) {
      (document as any).webkitExitFullscreen();
    }
  }, []);

  useEffect(() => {
    const onChange = () => {
      setIsFullscreen(!!document.fullscreenElement);
    };
    document.addEventListener('fullscreenchange', onChange);
    document.addEventListener('webkitfullscreenchange', onChange);
    return () => {
      document.removeEventListener('fullscreenchange', onChange);
      document.removeEventListener('webkitfullscreenchange', onChange);
    };
  }, []);

  const selectFile = useCallback((path: string) => {
    setCurrentFile(path);
    setSlides([]);
    setError('');
    setShowFileList(false);
    const url = new URL(location.href);
    url.searchParams.set('file', path);
    url.hash = '';
    history.pushState(null, '', url.toString());
  }, []);

  const backToList = useCallback(() => {
    if (contentMode) return;
    setCurrentFile('');
    setSlides([]);
    setMeta(null);
    setError('');
    setShowFileList(false);
    const url = new URL(location.href);
    url.searchParams.delete('file');
    url.hash = '';
    history.pushState(null, '', url.pathname + url.search);
  }, [contentMode]);

  useEffect(() => {
    const deck = deckRef.current;
    if (!deck || contentMode || !currentFile) return;

    const restorePosition = () => {
      const saved = localStorage.getItem(slidePositionKey(currentFile));
      if (!saved) {
        deck.slide(0, 0, 0);
        return;
      }

      try {
        const indices = JSON.parse(saved) as { h?: number; v?: number; f?: number };
        deck.slide(indices.h ?? 0, indices.v ?? 0, indices.f ?? 0);
      } catch {
        deck.slide(0, 0, 0);
      }
    };

    const savePosition = () => {
      const indices = deck.getIndices();
      localStorage.setItem(slidePositionKey(currentFile), JSON.stringify(indices));
    };

    restorePosition();
    deck.on('slidechanged', savePosition);
    deck.on('fragmentshown', savePosition);
    deck.on('fragmenthidden', savePosition);

    return () => {
      savePosition();
      deck.off('slidechanged', savePosition);
      deck.off('fragmentshown', savePosition);
      deck.off('fragmenthidden', savePosition);
    };
  }, [deckVersion, contentMode, currentFile]);

  useEffect(() => {
    const deck = deckRef.current;
    if (!deck) return;

    const applyOverviewStrip = () => {
      if (!deck.isOverview?.()) return;
      const revealEl = containerRef.current;
      const slidesEl = revealEl?.querySelector<HTMLElement>('.slides');
      if (!revealEl || !slidesEl) return;

      const horizontalSlides = Array.from(slidesEl.children).filter(
        child => (child as HTMLElement).tagName.toLowerCase() === 'section'
      ) as HTMLElement[];
      if (horizontalSlides.length === 0) return;

      const slideSize = deck.getComputedSlideSize();
      const overviewGap = 70;
      const overviewStep = slideSize.width + overviewGap;
      const overviewHeight = Math.min(420, Math.max(280, window.innerHeight * 0.42));
      const overviewWidth = Math.min(1280, Math.max(320, window.innerWidth - 96));
      const scale = Math.max(0.16, Math.min(0.32, (overviewHeight - 72) / slideSize.height));
      const visualWidth = horizontalSlides.length * overviewStep * scale;

      revealEl.style.overflowX = 'auto';
      revealEl.style.overflowY = 'hidden';
      revealEl.style.scrollBehavior = 'smooth';
      revealEl.style.position = 'fixed';
      revealEl.style.top = '50%';
      revealEl.style.left = '50%';
      revealEl.style.right = 'auto';
      revealEl.style.bottom = 'auto';
      revealEl.style.width = `${overviewWidth}px`;
      revealEl.style.height = `${overviewHeight}px`;
      revealEl.style.zIndex = '300';
      revealEl.style.overscrollBehavior = 'contain';
      revealEl.style.contain = 'layout paint size';
      revealEl.style.transform = 'translate(-50%, -50%)';
      revealEl.style.padding = '0 40px';
      slidesEl.style.left = '48px';
      slidesEl.style.top = '50%';
      slidesEl.style.width = `${visualWidth / scale}px`;
      slidesEl.style.height = `${slideSize.height}px`;
      slidesEl.style.transformOrigin = '0 50%';
      slidesEl.style.transform = `translateY(-50%) scale(${scale})`;

      const indices = deck.getIndices();
      const target = Math.max(0, indices.h * overviewStep * scale - overviewWidth / 2 + (slideSize.width * scale) / 2);
      revealEl.scrollLeft = target;
    };

    const resetOverviewStrip = () => {
      const revealEl = containerRef.current;
      const slidesEl = revealEl?.querySelector<HTMLElement>('.slides');
      if (revealEl) {
        revealEl.style.overflowX = '';
        revealEl.style.overflowY = '';
        revealEl.style.scrollBehavior = '';
        revealEl.style.position = '';
        revealEl.style.inset = '';
        revealEl.style.top = '';
        revealEl.style.left = '';
        revealEl.style.right = '';
        revealEl.style.bottom = '';
        revealEl.style.width = '';
        revealEl.style.height = '';
        revealEl.style.zIndex = '';
        revealEl.style.overscrollBehavior = '';
        revealEl.style.contain = '';
        revealEl.style.transform = '';
        revealEl.style.padding = '';
        revealEl.scrollLeft = 0;
      }
      if (slidesEl) {
        slidesEl.style.left = '';
        slidesEl.style.top = '';
        slidesEl.style.width = '';
        slidesEl.style.height = '';
        slidesEl.style.transformOrigin = '';
        slidesEl.style.transform = '';
      }
    };

    deck.on('overviewshown', applyOverviewStrip);
    deck.on('slidechanged', applyOverviewStrip);
    deck.on('overviewhidden', resetOverviewStrip);
    window.addEventListener('resize', applyOverviewStrip);

    const onOverviewWheel = (event: WheelEvent) => {
      if (!deck.isOverview?.()) return;
      const revealEl = containerRef.current;
      if (!revealEl) return;

      event.preventDefault();
      event.stopPropagation();
      revealEl.scrollLeft += Math.abs(event.deltaX) > Math.abs(event.deltaY) ? event.deltaX : event.deltaY;
    };

    containerRef.current?.addEventListener('wheel', onOverviewWheel, { passive: false });

    return () => {
      deck.off('overviewshown', applyOverviewStrip);
      deck.off('slidechanged', applyOverviewStrip);
      deck.off('overviewhidden', resetOverviewStrip);
      window.removeEventListener('resize', applyOverviewStrip);
      containerRef.current?.removeEventListener('wheel', onOverviewWheel);
      resetOverviewStrip();
    };
  }, [deckVersion, slides]);

  if (error) {
    return (
      <div className="slide-error">
        <h2>tt slide</h2>
        <pre>{error}</pre>
      </div>
    );
  }

  if (!contentMode && !currentFile && isListLoaded) {
    return (
      <div className="slide-list-page">
        <div className="slide-list-card">
          <div className="slide-list-title">tt slide</div>
          <div className="slide-list-subtitle">选择一个 slide 文档开始演示</div>
          {files.length === 0 ? (
            <div className="slide-list-empty">No .slide files found.</div>
          ) : (
            <div className="slide-list-grid">
              {files.map(f => (
                <button key={f.path} className="slide-list-card-item" onClick={() => selectFile(f.path)}>
                  <span className="slide-list-card-name">{f.name}</span>
                  <span className="slide-list-card-path">{f.path}</span>
                </button>
              ))}
            </div>
          )}
        </div>
      </div>
    );
  }

  if (slides.length === 0) {
    return (
      <div className="slide-loading">
        <div className="slide-loading-spinner" />
        <span>Loading slides...</span>
      </div>
    );
  }

  const tpl = getTemplate(templateOverride || meta?.template || DEFAULT_TEMPLATE);

  return (
    <div className="slide-wrapper" ref={wrapperRef}>
      <style>{tpl.css}</style>
      <div className={`reveal theme-${tpl.revealTheme}`} ref={containerRef}>
        <div className="slides">
          {slides.map((slide) => (
            <section key={slide.index} className={slide.class || ''}>
              <SlideContent slide={slide} theme={tpl.defaults.theme} />
            </section>
          ))}
        </div>
      </div>

      {files.length > 0 && (
        <div className={`slide-file-panel ${showFileList ? 'open' : ''}`}>
          <div className="slide-file-header">
            <span>Slides ({files.length})</span>
            <button className="slide-file-close" onClick={() => setShowFileList(false)}>×</button>
          </div>
          <div className="slide-file-list">
            {files.map(f => (
              <button
                key={f.path}
                className={`slide-file-item ${f.path === currentFile ? 'active' : ''}`}
                onClick={() => selectFile(f.path)}
              >
                {f.name}
              </button>
            ))}
          </div>
        </div>
      )}

      <button
        className="slide-fullscreen-btn"
        onClick={toggleFullscreen}
        title={isFullscreen ? 'Exit fullscreen (F)' : 'Fullscreen (F)'}
      >
        {isFullscreen ? '⊡' : '⛶'}
      </button>

      {!contentMode && currentFile && (
        <button
          className="slide-back-btn"
          onClick={backToList}
          title="Back to slide list"
        >
          ← 列表
        </button>
      )}

      {files.length > 0 && (
        <div className="slide-list-control">
          <button
            className="slide-list-btn"
            onClick={() => setShowFileList(v => !v)}
            title="File list (L)"
          >
            ☰
          </button>
        </div>
      )}
    </div>
  );
}
