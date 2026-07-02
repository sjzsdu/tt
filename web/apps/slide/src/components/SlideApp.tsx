import { useEffect, useRef, useState, useCallback } from 'react';
import type { DragEvent } from 'react';
import Reveal from 'reveal.js';
import { parseEditableSlideDocument, parseSlides, serializeEditableSlideDocument } from '../parser';
import { DEFAULT_TEMPLATE, getTemplate } from '../templates';
import type { SlideMeta, SlideRuntimeConfig, TemplateConfig, SlideWidgetRegistry } from '../types';
import { fetchSlideContent, fetchRawContent, fetchSlideList, fetchTemplate, fetchWidgets, createWS, saveSlideContent, rewriteSlide, type SlideFile } from '../api';
import { SlideContent } from './SlideContent';

const DESIGN_WIDTH = 1600;
const DESIGN_HEIGHT = 900;

function calculateStageScale() {
  const viewport = window.visualViewport;
  const viewportWidth = viewport?.width || window.innerWidth || DESIGN_WIDTH;
  const viewportHeight = viewport?.height || window.innerHeight || DESIGN_HEIGHT;
  return Math.min(viewportWidth / DESIGN_WIDTH, viewportHeight / DESIGN_HEIGHT);
}

const slidePositionKey = (file: string) => `tt-slide-position:${file}`;

function dirname(path: string) {
  const normalized = path.replace(/\\/g, '/');
  const index = normalized.lastIndexOf('/');
  if (index <= 0) return '';
  return normalized.slice(0, index);
}

interface SlideAppProps {
  contentMode: boolean;
  filePath?: string;
  templateOverride?: string;
  runtimeConfig?: SlideRuntimeConfig;
}

export function SlideApp({ contentMode, filePath, templateOverride = '', runtimeConfig = {} }: SlideAppProps) {
  const [slides, setSlides] = useState<Awaited<ReturnType<typeof parseSlides>>['slides']>([]);
  const [meta, setMeta] = useState<SlideMeta | null>(null);
  const [error, setError] = useState('');
  const [isFullscreen, setIsFullscreen] = useState(false);
  const [files, setFiles] = useState<SlideFile[]>([]);
  const [currentFile, setCurrentFile] = useState(filePath || '');
  const [showFileList, setShowFileList] = useState(false);
  const [showOverview, setShowOverview] = useState(false);
  const [isListLoaded, setIsListLoaded] = useState(contentMode);
  const [deckVersion, setDeckVersion] = useState(0);
  const [stageScale, setStageScale] = useState(1);
  const [rawMarkdown, setRawMarkdown] = useState('');
  const [currentSlideIndex, setCurrentSlideIndex] = useState(0);
  const [editorText, setEditorText] = useState('');
  const [editorMode, setEditorMode] = useState<'edit' | 'insert'>('edit');
  const [isEditorOpen, setIsEditorOpen] = useState(false);
  const [editorError, setEditorError] = useState('');
  const [isSaving, setIsSaving] = useState(false);
  const [isAIModalOpen, setIsAIModalOpen] = useState(false);
  const [aiInstruction, setAIInstruction] = useState('');
  const [aiError, setAIError] = useState('');
  const [isAIWorking, setIsAIWorking] = useState(false);
  const [deleteConfirmIndex, setDeleteConfirmIndex] = useState<number | null>(null);
  const [deleteError, setDeleteError] = useState('');
  const [isDeleting, setIsDeleting] = useState(false);
  const [draggedSlideIndex, setDraggedSlideIndex] = useState<number | null>(null);
  const [overviewError, setOverviewError] = useState('');
  const [isReordering, setIsReordering] = useState(false);
  const [template, setTemplate] = useState<TemplateConfig>(() => getTemplate(templateOverride || DEFAULT_TEMPLATE));
  const [widgets, setWidgets] = useState<SlideWidgetRegistry>({});
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

      setRawMarkdown(md);
      const { slides: parsed, meta: parsedMeta } = parseSlides(md, { assetBasePath: contentMode ? '' : dirname(file || '') });
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
    fetchWidgets()
      .then(res => setWidgets(res.widgets || {}))
      .catch(() => setWidgets({}));
  }, []);

  useEffect(() => {
    if (currentFile) {
      loadSlides(currentFile);
    } else if (contentMode) {
      loadSlides();
    }
  }, [currentFile, contentMode, loadSlides]);

  useEffect(() => {
    let cancelled = false;
    const name = templateOverride || DEFAULT_TEMPLATE;
    fetchTemplate(name)
      .then(tpl => {
        if (!cancelled) setTemplate(tpl);
      })
      .catch(() => {
        if (!cancelled) setTemplate(getTemplate(name));
      });
    return () => { cancelled = true; };
  }, [templateOverride]);

  useEffect(() => {
    if (slides.length === 0 || !containerRef.current) return;

    if (deckRef.current) {
      deckRef.current.destroy();
      deckRef.current = null;
    }

    const deck = new Reveal(containerRef.current, {
      hash: true,
      slideNumber: runtimeConfig.slideNumber ?? true,
      controls: runtimeConfig.controls ?? true,
      progress: runtimeConfig.progress ?? true,
      overview: runtimeConfig.overview ?? false,
      center: runtimeConfig.center ?? template.defaults.center,
      transition: runtimeConfig.transition || meta?.transition || template.defaults.transition,
      autoSlide: runtimeConfig.autoSlide ?? 0,
      disableLayout: true,
      margin: runtimeConfig.margin ?? template.defaults.margin ?? 0.04,
      minScale: 1,
      maxScale: 1,
    });

    deck.initialize().then(() => {
      deckRef.current = deck;
      setDeckVersion(v => v + 1);
    });

    return () => {
      deck.destroy();
      deckRef.current = null;
    };
  }, [slides, meta, runtimeConfig, template]);

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
    const id = 'tt-slide-widget-css';
    let style = document.getElementById(id) as HTMLStyleElement | null;
    if (!style) {
      style = document.createElement('style');
      style.id = id;
      document.head.appendChild(style);
    }
    style.textContent = Object.values(widgets).map(widget => widget.css || '').filter(Boolean).join('\n\n');
  }, [widgets]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) return;
      if (e.key === 'f' || e.key === 'F') {
        e.preventDefault();
        toggleFullscreen();
      }
      if (e.key === 'Escape' && isFullscreen) {
        exitFullscreen();
        return;
      }
      if (e.key === 'Escape') {
        e.preventDefault();
        setShowOverview(v => !v);
        return;
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

  useEffect(() => {
    const updateStageScale = () => {
      setStageScale(calculateStageScale());
      deckRef.current?.layout();
    };

    updateStageScale();
    window.addEventListener('resize', updateStageScale);
    window.visualViewport?.addEventListener('resize', updateStageScale);
    document.addEventListener('fullscreenchange', updateStageScale);
    document.addEventListener('webkitfullscreenchange', updateStageScale);
    return () => {
      window.removeEventListener('resize', updateStageScale);
      window.visualViewport?.removeEventListener('resize', updateStageScale);
      document.removeEventListener('fullscreenchange', updateStageScale);
      document.removeEventListener('webkitfullscreenchange', updateStageScale);
    };
  }, []);

  const selectFile = useCallback((path: string) => {
    setCurrentFile(path);
    setSlides([]);
    setError('');
    setShowFileList(false);
    setShowOverview(false);
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
    setRawMarkdown('');
    setCurrentSlideIndex(0);
    setShowFileList(false);
    setShowOverview(false);
    const url = new URL(location.href);
    url.searchParams.delete('file');
    url.hash = '';
    history.pushState(null, '', url.pathname + url.search);
  }, [contentMode]);

  const closeOverviewFromStage = useCallback(() => {
    if (showOverview) {
      setShowOverview(false);
    }
  }, [showOverview]);

  const getActiveSlideIndex = useCallback(() => {
    const indices = deckRef.current?.getIndices();
    return Math.max(0, Math.min(indices?.h ?? currentSlideIndex, Math.max(slides.length - 1, 0)));
  }, [currentSlideIndex, slides.length]);

  const saveEditableDocument = useCallback(async (doc: ReturnType<typeof parseEditableSlideDocument>, nextIndex: number) => {
    if (!currentFile) throw new Error('No slide file specified');
    const nextMarkdown = serializeEditableSlideDocument(doc);
    const safeIndex = Math.max(0, Math.min(nextIndex, Math.max(doc.slides.length - 1, 0)));
    await saveSlideContent(currentFile, nextMarkdown);
    localStorage.setItem(slidePositionKey(currentFile), JSON.stringify({ h: safeIndex, v: 0, f: 0 }));
    setRawMarkdown(nextMarkdown);
    const { slides: parsed, meta: parsedMeta } = parseSlides(nextMarkdown, { assetBasePath: dirname(currentFile) });
    setSlides(parsed);
    setMeta(parsedMeta);
    setCurrentSlideIndex(safeIndex);
    setTimeout(() => deckRef.current?.slide(safeIndex, 0, 0), 0);
  }, [currentFile]);

  const openCurrentSlideEditor = useCallback(() => {
    if (contentMode || !currentFile || !rawMarkdown) return;
    const doc = parseEditableSlideDocument(rawMarkdown);
    const index = getActiveSlideIndex();
    const raw = doc.slides[index];
    if (raw == null) {
      setEditorError(`Cannot find source for slide ${index + 1}`);
      return;
    }
    setCurrentSlideIndex(index);
    setEditorText(raw);
    setEditorMode('edit');
    setEditorError('');
    setIsEditorOpen(true);
  }, [contentMode, currentFile, getActiveSlideIndex, rawMarkdown]);

  const openInsertSlideEditor = useCallback(() => {
    if (contentMode || !currentFile || !rawMarkdown) return;
    const doc = parseEditableSlideDocument(rawMarkdown);
    const activeIndex = getActiveSlideIndex();
    const insertIndex = Math.max(0, Math.min(activeIndex + 1, doc.slides.length));
    setCurrentSlideIndex(insertIndex);
    setEditorMode('insert');
    setEditorText('# 新页面\n\n- 在这里输入这一页的要点\n- 可以使用 Markdown、Mermaid、D2 或 widget');
    setEditorError('将在当前页后插入新 slide。');
    setIsEditorOpen(true);
  }, [contentMode, currentFile, getActiveSlideIndex, rawMarkdown]);

  const openAIRewrite = useCallback(() => {
    if (contentMode || !currentFile || !rawMarkdown) return;
    const doc = parseEditableSlideDocument(rawMarkdown);
    const index = getActiveSlideIndex();
    if (doc.slides[index] == null) {
      setAIError(`Cannot find source for slide ${index + 1}`);
      return;
    }
    setCurrentSlideIndex(index);
    setAIInstruction('');
    setAIError('');
    setIsAIModalOpen(true);
  }, [contentMode, currentFile, getActiveSlideIndex, rawMarkdown]);

  const submitAIRewrite = useCallback(async () => {
    if (contentMode || !currentFile) return;
    const instruction = aiInstruction.trim();
    if (!instruction) {
      setAIError('请输入修改意见。');
      return;
    }
    const doc = parseEditableSlideDocument(rawMarkdown);
    const index = currentSlideIndex;
    const slideSource = doc.slides[index];
    if (slideSource == null) {
      setAIError(`Cannot find source for slide ${index + 1}`);
      return;
    }
    setIsAIWorking(true);
    setAIError('');
    try {
      const response = await rewriteSlide({
        file: currentFile,
        slideIndex: index,
        slideSource,
        instruction,
        previousSlide: doc.slides[index - 1],
        nextSlide: doc.slides[index + 1],
      });
      const updated = response.updatedSlideSource?.trim();
      if (!updated) throw new Error(response.error || 'slide-writer returned empty content');
      setEditorText(updated);
      setEditorError(response.summary || 'AI 已生成修改稿，请确认后保存。');
      setIsAIModalOpen(false);
      setIsEditorOpen(true);
    } catch (e: any) {
      setAIError(String(e?.message || e));
    } finally {
      setIsAIWorking(false);
    }
  }, [aiInstruction, contentMode, currentFile, currentSlideIndex, rawMarkdown]);

  const saveCurrentSlide = useCallback(async () => {
    if (contentMode || !currentFile) return;
    const doc = parseEditableSlideDocument(rawMarkdown);
    const index = currentSlideIndex;
    if (editorMode === 'edit' && (index < 0 || index >= doc.slides.length)) {
      setEditorError(`Cannot find source for slide ${index + 1}`);
      return;
    }
    const nextText = editorText.trim();
    if (!nextText) {
      setEditorError('Slide content cannot be empty. Use Delete to remove a slide.');
      return;
    }
    if (editorMode === 'insert') {
      const insertIndex = Math.max(0, Math.min(index, doc.slides.length));
      doc.slides.splice(insertIndex, 0, nextText);
    } else {
      doc.slides[index] = nextText;
    }
    setIsSaving(true);
    setEditorError('');
    try {
      await saveEditableDocument(doc, index);
      setIsEditorOpen(false);
    } catch (e: any) {
      setEditorError(String(e?.message || e));
    } finally {
      setIsSaving(false);
    }
  }, [contentMode, currentFile, currentSlideIndex, editorMode, editorText, rawMarkdown, saveEditableDocument]);

  const requestDeleteCurrentSlide = useCallback(() => {
    if (contentMode || !currentFile || !rawMarkdown) return;
    const doc = parseEditableSlideDocument(rawMarkdown);
    const index = getActiveSlideIndex();
    if (doc.slides.length <= 1) {
      setDeleteError('当前文档只有 1 页，不能删除最后一页。');
      setDeleteConfirmIndex(index);
      return;
    }
    setDeleteError('');
    setDeleteConfirmIndex(index);
  }, [contentMode, currentFile, getActiveSlideIndex, rawMarkdown]);

  const confirmDeleteCurrentSlide = useCallback(async () => {
    if (contentMode || !currentFile || !rawMarkdown || deleteConfirmIndex == null) return;
    const doc = parseEditableSlideDocument(rawMarkdown);
    const index = Math.max(0, Math.min(deleteConfirmIndex, doc.slides.length - 1));
    if (doc.slides.length <= 1) {
      setDeleteError('当前文档只有 1 页，不能删除最后一页。');
      return;
    }
    doc.slides.splice(index, 1);
    const nextIndex = Math.max(0, Math.min(index, doc.slides.length - 1));
    setIsDeleting(true);
    setDeleteError('');
    try {
      await saveEditableDocument(doc, nextIndex);
      setDeleteConfirmIndex(null);
    } catch (e: any) {
      setDeleteError(`删除失败：${String(e?.message || e)}`);
    } finally {
      setIsDeleting(false);
    }
  }, [contentMode, currentFile, deleteConfirmIndex, rawMarkdown, saveEditableDocument]);

  const reorderSlides = useCallback(async (fromIndex: number, toIndex: number) => {
    if (contentMode || !currentFile || !rawMarkdown || fromIndex === toIndex) return;
    const doc = parseEditableSlideDocument(rawMarkdown);
    if (fromIndex < 0 || fromIndex >= doc.slides.length || toIndex < 0 || toIndex >= doc.slides.length) return;
    const [moved] = doc.slides.splice(fromIndex, 1);
    doc.slides.splice(toIndex, 0, moved);
    setIsReordering(true);
    setOverviewError('');
    try {
      await saveEditableDocument(doc, toIndex);
    } catch (e: any) {
      setOverviewError(`排序保存失败：${String(e?.message || e)}`);
    } finally {
      setIsReordering(false);
      setDraggedSlideIndex(null);
    }
  }, [contentMode, currentFile, rawMarkdown, saveEditableDocument]);

  const handleOverviewDragStart = useCallback((event: DragEvent<HTMLDivElement>, index: number) => {
    setDraggedSlideIndex(index);
    setOverviewError('');
    event.dataTransfer.effectAllowed = 'move';
    event.dataTransfer.setData('text/plain', String(index));
  }, []);

  const handleOverviewDrop = useCallback((event: DragEvent<HTMLDivElement>, index: number) => {
    event.preventDefault();
    event.stopPropagation();
    const rawIndex = event.dataTransfer.getData('text/plain');
    const fromIndex = rawIndex ? Number(rawIndex) : draggedSlideIndex;
    if (Number.isInteger(fromIndex)) {
      void reorderSlides(fromIndex as number, index);
    }
  }, [draggedSlideIndex, reorderSlides]);

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
	  setCurrentSlideIndex(indices.h ?? 0);
	  localStorage.setItem(slidePositionKey(currentFile), JSON.stringify(indices));
	};

	restorePosition();
	setCurrentSlideIndex(deck.getIndices().h ?? 0);
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

  const tpl = template;

  return (
    <div className="slide-wrapper" ref={wrapperRef}>
      <style>{tpl.css}</style>
      <div
        className="slide-stage"
        style={{ transform: `scale(${stageScale})` }}
        aria-label="16:9 presentation stage"
      >
        <div className={`reveal theme-${tpl.revealTheme}`} ref={containerRef} onClick={closeOverviewFromStage}>
          <div className="slides">
            {slides.map((slide) => (
              <section key={slide.index} className={slide.class || ''}>
                <SlideContent slide={slide} theme={tpl.defaults.theme} widgets={widgets} />
              </section>
            ))}
          </div>
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

      {showOverview && (
        <div className="slide-overview-modal" role="dialog" aria-label="Slide overview">
          <div className="slide-overview-header">
            <div>
              <span>Slides</span>
              {!contentMode && currentFile && <small>拖拽缩略图可排序</small>}
            </div>
            <button className="slide-overview-close" onClick={() => setShowOverview(false)} title="Close overview">×</button>
          </div>
          {overviewError && <div className="slide-overview-error">{overviewError}</div>}
          {isReordering && <div className="slide-overview-saving">正在保存排序…</div>}
          <div className="slide-overview-list">
            {slides.map((slide) => {
              const isActive = deckRef.current?.getIndices().h === slide.index;
              const isDragging = draggedSlideIndex === slide.index;
              return (
                <div
                  key={slide.index}
                  role="button"
                  tabIndex={0}
                  draggable={!contentMode && !!currentFile && !isReordering}
                  className={`slide-overview-item ${isActive ? 'active' : ''} ${isDragging ? 'dragging' : ''} ${draggedSlideIndex != null && !isDragging ? 'drop-ready' : ''}`}
                  onDragStart={(event) => handleOverviewDragStart(event, slide.index)}
                  onDragOver={(event) => {
                    if (draggedSlideIndex == null || draggedSlideIndex === slide.index) return;
                    event.preventDefault();
                    event.dataTransfer.dropEffect = 'move';
                  }}
                  onDrop={(event) => handleOverviewDrop(event, slide.index)}
                  onDragEnd={() => setDraggedSlideIndex(null)}
                  onClick={() => {
                    if (draggedSlideIndex != null) return;
                    deckRef.current?.slide(slide.index, 0, 0);
                    setShowOverview(false);
                  }}
                  onKeyDown={(event) => {
                    if (event.key !== 'Enter' && event.key !== ' ') return;
                    event.preventDefault();
                    deckRef.current?.slide(slide.index, 0, 0);
                    setShowOverview(false);
                  }}
                >
                  <span className="slide-overview-number">{slide.index + 1}</span>
                  <div className={`slide-overview-thumb theme-${tpl.revealTheme}`}>
                    <div className="slide-overview-thumb-inner reveal">
                      <div className="slides">
                        {Array.from({ length: slide.index }).map((_, placeholderIndex) => (
                          <section
                            key={`placeholder-${placeholderIndex}`}
                            className="slide-overview-placeholder"
                            aria-hidden="true"
                          />
                        ))}
                        <section className={`${slide.class || ''} present`.trim()}>
                          <SlideContent slide={slide} theme={tpl.defaults.theme} widgets={widgets} />
                        </section>
                      </div>
                    </div>
                  </div>
                </div>
              );
            })}
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

      {!contentMode && currentFile && !isFullscreen && (
        <div className="slide-toolbar-hotspot" aria-label="Slide actions">
          <div className="slide-toolbar">
            <button className="slide-toolbar-btn" onClick={backToList} title="Back to slide list">
              ← 列表
            </button>
            {files.length > 0 && (
              <button className="slide-toolbar-btn" onClick={() => setShowFileList(v => !v)} title="File list (L)">
                ☰ 文档
              </button>
            )}
            <button className="slide-toolbar-btn" onClick={openInsertSlideEditor} title="Insert a new slide after current slide">
              ＋ 插入
            </button>
            <button className="slide-toolbar-btn" onClick={openCurrentSlideEditor} title="Edit current slide">
              ✎ 编辑
            </button>
            <button className="slide-toolbar-btn" onClick={openAIRewrite} title="Ask slide-writer to revise current slide">
              💬 AI 修改
            </button>
            <button className="slide-toolbar-btn danger" onClick={requestDeleteCurrentSlide} title="Delete current slide">
              🗑 删除
            </button>
          </div>
        </div>
      )}

      {isEditorOpen && (
        <div className="slide-edit-modal" role="dialog" aria-modal="true" aria-label="Edit current slide">
          <div className="slide-edit-dialog">
            <div className="slide-edit-header">
              <div>
                <div className="slide-edit-title">{editorMode === 'insert' ? `插入第 ${currentSlideIndex + 1} 页` : `编辑第 ${currentSlideIndex + 1} 页`}</div>
                <div className="slide-edit-subtitle">{editorMode === 'insert' ? `${currentFile} · 新页面将插入到当前位置` : currentFile}</div>
              </div>
              <button className="slide-edit-close" onClick={() => setIsEditorOpen(false)} title="Close">×</button>
            </div>
            <textarea
              className="slide-edit-textarea"
              value={editorText}
              onChange={(event) => setEditorText(event.target.value)}
              spellCheck={false}
              autoFocus
            />
            {editorError && <div className="slide-edit-error">{editorError}</div>}
            <div className="slide-edit-actions">
              <button className="slide-edit-secondary" onClick={() => setIsEditorOpen(false)} disabled={isSaving}>取消</button>
              <button className="slide-edit-primary" onClick={saveCurrentSlide} disabled={isSaving}>
                {isSaving ? '保存中…' : editorMode === 'insert' ? '插入并保存' : '保存'}
              </button>
            </div>
          </div>
        </div>
      )}

      {deleteConfirmIndex != null && (
        <div className="slide-edit-modal" role="dialog" aria-modal="true" aria-label="Delete current slide">
          <div className="slide-delete-dialog">
            <div className="slide-delete-icon">🗑</div>
            <div className="slide-delete-title">删除第 {deleteConfirmIndex + 1} 页？</div>
            <div className="slide-delete-message">
              这会从源文件 <code>{currentFile}</code> 中移除当前页内容。删除后会自动保存并刷新预览。
            </div>
            {deleteError && <div className="slide-edit-error">{deleteError}</div>}
            <div className="slide-edit-actions slide-delete-actions">
              <button className="slide-edit-secondary" onClick={() => setDeleteConfirmIndex(null)} disabled={isDeleting}>取消</button>
              <button className="slide-edit-danger" onClick={confirmDeleteCurrentSlide} disabled={isDeleting || !!deleteError && slides.length <= 1}>
                {isDeleting ? '删除中…' : '确认删除'}
              </button>
            </div>
          </div>
        </div>
      )}

      {isAIModalOpen && (
        <div className="slide-edit-modal" role="dialog" aria-modal="true" aria-label="AI revise current slide">
          <div className="slide-ai-dialog">
            <div className="slide-edit-header">
              <div>
                <div className="slide-edit-title">AI 修改第 {currentSlideIndex + 1} 页</div>
                <div className="slide-edit-subtitle">输入修改意见，slide-writer 会生成当前页修改稿</div>
              </div>
              <button className="slide-edit-close" onClick={() => setIsAIModalOpen(false)} title="Close">×</button>
            </div>
            <textarea
              className="slide-ai-textarea"
              value={aiInstruction}
              onChange={(event) => setAIInstruction(event.target.value)}
              placeholder="例如：这页文字太多，请明显删减左侧文字，保留左右图文结构，把右侧 Mermaid 改成更像成长曲线。"
              disabled={isAIWorking}
              autoFocus
            />
            {aiError && <div className="slide-edit-error">{aiError}</div>}
            <div className="slide-edit-actions">
              <button className="slide-edit-secondary" onClick={() => setIsAIModalOpen(false)} disabled={isAIWorking}>取消</button>
              <button className="slide-edit-primary" onClick={submitAIRewrite} disabled={isAIWorking}>
                {isAIWorking ? '生成中…' : '生成修改稿'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
