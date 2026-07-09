import { useEffect, useRef, useState, useCallback } from 'react';
import type { DragEvent, MouseEvent } from 'react';
import Reveal from 'reveal.js';
import { parseEditableSlideDocument, parseSlides, serializeEditableSlideDocument } from '../parser';
import { DEFAULT_TEMPLATE, getTemplate } from '../templates';
import type { SlideMeta, SlideRuntimeConfig, TemplateConfig, SlideWidgetRegistry } from '../types';
import { fetchSlideContent, fetchRawContent, fetchSlideList, fetchTemplate, fetchWidgets, createWS, saveSlideContent, rewriteSlide, openFormulaRun, openFormulaShow, openMarkdownFile, type SlideFile } from '../api';
import { SlideContent } from './SlideContent';

const DESIGN_WIDTH = 1600;
const DESIGN_HEIGHT = 900;

type AIRewriteTurn = {
  instruction: string;
  summary: string;
  before: string;
  after: string;
};

const CORE_LAYOUT_CSS = `
.reveal .slides section.slide-grid .slide-content,
.reveal .slides section.slide-cards .slide-content {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
  gap: 24px;
  align-content: start;
  height: 100%;
}
.reveal .slides section.slide-grid.slide-cols-2 .slide-content { grid-template-columns: repeat(2, minmax(0, 1fr)); }
.reveal .slides section.slide-grid.slide-cols-3 .slide-content { grid-template-columns: repeat(3, minmax(0, 1fr)); }
.reveal .slides section.slide-grid.slide-cols-4 .slide-content { grid-template-columns: repeat(4, minmax(0, 1fr)); }
.reveal .slides section.slide-grid.slide-cols-5 .slide-content { grid-template-columns: repeat(5, minmax(0, 1fr)); }
.reveal .slides section.slide-grid.slide-cols-6 .slide-content { grid-template-columns: repeat(6, minmax(0, 1fr)); }
.reveal .slides section.slide-grid.slide-rows-2 .slide-content { grid-template-rows: auto repeat(2, minmax(0, 1fr)); align-content: stretch; align-items: stretch; }
.reveal .slides section.slide-grid.slide-rows-3 .slide-content { grid-template-rows: auto repeat(3, minmax(0, 1fr)); align-content: stretch; align-items: stretch; }
.reveal .slides section.slide-grid.slide-compact .slide-content { gap: 14px; }
.reveal .slides section.slide-grid.slide-dense .slide-content { gap: 10px; }
.reveal .slides section.slide-grid .slide-markdown:not(.slide-part-item):not(.slide-part-card),
.reveal .slides section.slide-cards .slide-markdown:not(.slide-part-item):not(.slide-part-card) { grid-column: 1 / -1; }
.reveal .slides section.slide-grid.slide-compact .slide-markdown:not(.slide-part-item):not(.slide-part-card) h1,
.reveal .slides section.slide-grid.slide-dense .slide-markdown:not(.slide-part-item):not(.slide-part-card) h1 { margin-bottom: 0; font-size: 1.28em; }
.reveal .slides section.slide-grid.slide-compact .slide-widget,
.reveal .slides section.slide-grid.slide-dense .slide-widget { min-width: 0; min-height: 0; height: 100%; }
`;

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

function actionValueFromHref(href: string, scheme: string) {
  const trimmed = href.trim();
  if (!trimmed.toLowerCase().startsWith(scheme)) return '';
  let value = trimmed.slice(scheme.length);
  if (value.startsWith('//')) value = value.slice(2);
  value = value.replace(/^\/+/, '');
  try {
    return decodeURIComponent(value);
  } catch {
    return value;
  }
}

function formulaRunIDFromHref(href: string) {
  return actionValueFromHref(href, 'tt-formula-run:');
}

function formulaShowNameFromHref(href: string) {
  return actionValueFromHref(href, 'tt-formula-show:')
    || actionValueFromHref(href, 'tt-formula://show/')
    || actionValueFromHref(href, 'tt-formula:show/');
}

function markdownPathFromHref(href: string) {
  return actionValueFromHref(href, 'tt-md:') || actionValueFromHref(href, 'tt-markdown:');
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
  const [editorSlideIndex, setEditorSlideIndex] = useState<number | null>(null);
  const [editorFilePath, setEditorFilePath] = useState('');
  const [editorBaseMarkdown, setEditorBaseMarkdown] = useState('');
  const [editorText, setEditorText] = useState('');
  const [editorMode, setEditorMode] = useState<'edit' | 'insert'>('edit');
  const [isEditorOpen, setIsEditorOpen] = useState(false);
  const [editorError, setEditorError] = useState('');
  const [isSaving, setIsSaving] = useState(false);
  const [isAIModalOpen, setIsAIModalOpen] = useState(false);
  const [aiInstruction, setAIInstruction] = useState('');
  const [aiError, setAIError] = useState('');
  const [isAIWorking, setIsAIWorking] = useState(false);
  const [aiRewriteTurns, setAIRewriteTurns] = useState<AIRewriteTurn[]>([]);
  const [deleteConfirmIndex, setDeleteConfirmIndex] = useState<number | null>(null);
  const [deleteError, setDeleteError] = useState('');
  const [isDeleting, setIsDeleting] = useState(false);
  const [draggedSlideIndex, setDraggedSlideIndex] = useState<number | null>(null);
  const [overviewError, setOverviewError] = useState('');
  const [isReordering, setIsReordering] = useState(false);
  const [template, setTemplate] = useState<TemplateConfig>(() => getTemplate(templateOverride || DEFAULT_TEMPLATE));
  const [widgets, setWidgets] = useState<SlideWidgetRegistry>({});
  const [actionNotice, setActionNotice] = useState<{ kind: 'info' | 'success' | 'error'; text: string } | null>(null);
  const deckRef = useRef<Reveal.Api | null>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const wrapperRef = useRef<HTMLDivElement>(null);
  const overviewListRef = useRef<HTMLDivElement>(null);
  const currentFileRef = useRef(currentFile);
  currentFileRef.current = currentFile;

  const exposeCaptureAPI = useCallback((deck: Reveal.Api | null) => {
    (window as any).ttSlideCapture = {
      ready: Boolean(deck),
      slideCount: slides.length,
      goTo: async (index: number) => {
        if (!deck) throw new Error('slide deck is not ready');
        const clamped = Math.max(0, Math.min(slides.length - 1, Number(index) || 0));
        deck.slide(clamped);
        await new Promise(resolve => window.setTimeout(resolve, 350));
        return deck.getIndices();
      },
      current: () => deck ? deck.getIndices() : null,
    };
  }, [slides.length]);

  const loadSlides = useCallback(async (file?: string) => {
    try {
      let md: string;
      if (contentMode) {
        md = await fetchRawContent();
      } else if (file) {
        md = await fetchSlideContent(file);
        if (file !== currentFileRef.current) return;
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
      hideInactiveCursor: false,
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
      exposeCaptureAPI(deck);
      setDeckVersion(v => v + 1);
    });

    return () => {
      exposeCaptureAPI(null);
      deck.destroy();
      deckRef.current = null;
    };
  }, [slides, meta, runtimeConfig, template, exposeCaptureAPI]);

  useEffect(() => {
    if (contentMode) return;
    const fileForSocket = currentFile;
    const ws = createWS((data) => {
      if (data.type === 'reload') {
        loadSlides(fileForSocket);
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

  useEffect(() => {
    if (!showOverview) return;
    const timeout = window.setTimeout(() => {
      const activeIndex = deckRef.current?.getIndices().h ?? currentSlideIndex;
      const activeItem = overviewListRef.current?.querySelector<HTMLElement>(`[data-slide-index="${activeIndex}"]`);
      activeItem?.scrollIntoView({ block: 'center', inline: 'nearest' });
    }, 0);
    return () => window.clearTimeout(timeout);
  }, [showOverview, currentSlideIndex]);

  const selectFile = useCallback((path: string) => {
    setCurrentFile(path);
    setSlides([]);
    setError('');
    setIsEditorOpen(false);
    setIsAIModalOpen(false);
    setEditorSlideIndex(null);
    setEditorFilePath('');
    setEditorBaseMarkdown('');
    setAIInstruction('');
    setAIError('');
    setAIRewriteTurns([]);
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
    setEditorSlideIndex(null);
    setEditorFilePath('');
    setEditorBaseMarkdown('');
    setIsEditorOpen(false);
    setIsAIModalOpen(false);
    setAIInstruction('');
    setAIError('');
    setAIRewriteTurns([]);
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

  const handleStageClick = useCallback(async (event: MouseEvent<HTMLDivElement>) => {
    const target = event.target instanceof Element ? event.target : null;
    const anchor = target?.closest('a[href]') as HTMLAnchorElement | null;
    const href = anchor?.getAttribute('href') || '';
    const runId = formulaRunIDFromHref(href);
    const formulaName = formulaShowNameFromHref(href);
    const markdownPath = markdownPathFromHref(href);
    if (!runId && !formulaName && !markdownPath) {
      closeOverviewFromStage();
      return;
    }

    event.preventDefault();
    event.stopPropagation();
    if (runId) {
      setActionNotice({ kind: 'info', text: `正在打开 formula run：${runId}` });
      try {
        const result = await openFormulaRun(runId);
        setActionNotice({ kind: 'success', text: `已打开 formula run：${result.runId}` });
      } catch (e: any) {
        setActionNotice({ kind: 'error', text: `打开失败：${String(e?.message || e)}` });
      }
      return;
    }

    if (formulaName) {
      setActionNotice({ kind: 'info', text: `正在打开 formula：${formulaName}` });
      try {
        const result = await openFormulaShow(formulaName);
        setActionNotice({ kind: 'success', text: `已打开 formula：${result.name}` });
      } catch (e: any) {
        setActionNotice({ kind: 'error', text: `打开失败：${String(e?.message || e)}` });
      }
      return;
    }

    setActionNotice({ kind: 'info', text: `正在打开 Markdown：${markdownPath}` });
    try {
      const result = await openMarkdownFile(markdownPath);
      setActionNotice({ kind: 'success', text: `已打开 Markdown：${result.path}` });
    } catch (e: any) {
      setActionNotice({ kind: 'error', text: `打开失败：${String(e?.message || e)}` });
    }
  }, [closeOverviewFromStage]);

  const getActiveSlideIndex = useCallback(() => {
    const indices = deckRef.current?.getIndices();
    return Math.max(0, Math.min(indices?.h ?? currentSlideIndex, Math.max(slides.length - 1, 0)));
  }, [currentSlideIndex, slides.length]);

  const editorDisplayIndex = editorSlideIndex ?? currentSlideIndex;

  const closeEditor = useCallback(() => {
    setIsEditorOpen(false);
    setIsAIModalOpen(false);
    setEditorSlideIndex(null);
    setEditorFilePath('');
    setEditorBaseMarkdown('');
    setAIInstruction('');
    setAIError('');
    setAIRewriteTurns([]);
  }, []);

  const saveEditableDocument = useCallback(async (doc: ReturnType<typeof parseEditableSlideDocument>, nextIndex: number, targetFile = currentFile) => {
    if (!targetFile) throw new Error('No slide file specified');
    const nextMarkdown = serializeEditableSlideDocument(doc);
    const safeIndex = Math.max(0, Math.min(nextIndex, Math.max(doc.slides.length - 1, 0)));
    await saveSlideContent(targetFile, nextMarkdown);
    localStorage.setItem(slidePositionKey(targetFile), JSON.stringify({ h: safeIndex, v: 0, f: 0 }));
    if (targetFile !== currentFileRef.current) return;
    setRawMarkdown(nextMarkdown);
    const { slides: parsed, meta: parsedMeta } = parseSlides(nextMarkdown, { assetBasePath: dirname(targetFile) });
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
    setEditorSlideIndex(index);
    setEditorFilePath(currentFile);
    setEditorBaseMarkdown(rawMarkdown);
    setEditorText(raw);
    setEditorMode('edit');
    setEditorError('');
    setAIInstruction('');
    setAIError('');
    setAIRewriteTurns([]);
    setIsAIModalOpen(false);
    setIsEditorOpen(true);
  }, [contentMode, currentFile, getActiveSlideIndex, rawMarkdown]);

  const openInsertSlideEditor = useCallback(() => {
    if (contentMode || !currentFile || !rawMarkdown) return;
    const doc = parseEditableSlideDocument(rawMarkdown);
    const activeIndex = getActiveSlideIndex();
    const insertIndex = Math.max(0, Math.min(activeIndex + 1, doc.slides.length));
    setCurrentSlideIndex(insertIndex);
    setEditorSlideIndex(insertIndex);
    setEditorFilePath(currentFile);
    setEditorBaseMarkdown(rawMarkdown);
    setEditorMode('insert');
    setEditorText('# 新页面\n\n- 在这里输入这一页的要点\n- 可以使用 Markdown、Mermaid、D2 或 widget');
    setEditorError('将在当前页后插入新 slide。');
    setAIInstruction('');
    setAIError('');
    setAIRewriteTurns([]);
    setIsAIModalOpen(false);
    setIsEditorOpen(true);
  }, [contentMode, currentFile, getActiveSlideIndex, rawMarkdown]);

  const openAIRewrite = useCallback(() => {
    if (contentMode || !currentFile || !rawMarkdown) return;
    const doc = parseEditableSlideDocument(rawMarkdown);
    const index = getActiveSlideIndex();
    const raw = doc.slides[index];
    if (raw == null) {
      setAIError(`Cannot find source for slide ${index + 1}`);
      return;
    }
    setCurrentSlideIndex(index);
    setEditorSlideIndex(index);
    setEditorFilePath(currentFile);
    setEditorBaseMarkdown(rawMarkdown);
    setEditorText(raw);
    setEditorMode('edit');
    setEditorError('');
    setAIInstruction('');
    setAIError('');
    setAIRewriteTurns([]);
    setIsEditorOpen(true);
    setIsAIModalOpen(true);
  }, [contentMode, currentFile, getActiveSlideIndex, rawMarkdown]);

  const openAIRewriteFromEditor = useCallback(() => {
    if (contentMode || !currentFile || !rawMarkdown) return;
    if (!editorFilePath) setEditorFilePath(currentFile);
    if (!editorBaseMarkdown) setEditorBaseMarkdown(rawMarkdown);
    const draft = editorText.trim();
    if (!draft) {
      setEditorError('当前编辑内容为空，无法进行 AI 修改。');
      return;
    }
    setAIInstruction('');
    setAIError('');
    setIsEditorOpen(true);
    setIsAIModalOpen(true);
  }, [contentMode, currentFile, editorBaseMarkdown, editorFilePath, editorText, rawMarkdown]);

  const submitAIRewrite = useCallback(async () => {
    const targetFile = editorFilePath || currentFile;
    if (contentMode || !targetFile) return;
    const instruction = aiInstruction.trim();
    if (!instruction) {
      setAIError('请输入修改意见。');
      return;
    }
    const sourceMarkdown = editorBaseMarkdown || rawMarkdown;
    const doc = parseEditableSlideDocument(sourceMarkdown);
    const index = editorSlideIndex ?? currentSlideIndex;
    const useEditorDraft = isEditorOpen && editorText.trim() !== '';
    const slideSource = useEditorDraft ? editorText.trim() : doc.slides[index];
    if (!slideSource) {
      setAIError(`Cannot find source for slide ${index + 1}`);
      return;
    }
    if (useEditorDraft && index >= 0 && index < doc.slides.length) {
      doc.slides[index] = slideSource;
    }
    setIsAIWorking(true);
    setAIError('');
    try {
      const sessionInstruction = aiRewriteTurns.length > 0
        ? `这是一个多轮改稿会话。请以“当前页源码”为最新草稿继续修改，不要恢复旧稿。\n\n历史修改意见与结果摘要：\n${aiRewriteTurns.map((turn, turnIndex) => `${turnIndex + 1}. 用户：${turn.instruction}\n   结果：${turn.summary}`).join('\n')}\n\n本轮用户意见：\n${instruction}`
        : instruction;
      const response = await rewriteSlide({
        file: targetFile,
        slideIndex: index,
        slideSource,
        instruction: sessionInstruction,
        previousSlide: doc.slides[index - 1],
        nextSlide: editorMode === 'insert' ? doc.slides[index] : doc.slides[index + 1],
      });
      const updated = response.updatedSlideSource?.trim();
      if (!updated) throw new Error(response.error || 'slide-writer returned empty content');
      const summary = response.summary || 'AI 已生成一版修改稿。';
      setEditorText(updated);
      setEditorError(`${summary} 可以继续输入意见迭代，满意后再保存。`);
      setAIRewriteTurns(turns => [...turns, { instruction, summary, before: slideSource, after: updated }]);
      setAIInstruction('');
      setIsEditorOpen(true);
    } catch (e: any) {
      setAIError(String(e?.message || e));
    } finally {
      setIsAIWorking(false);
    }
  }, [aiInstruction, aiRewriteTurns, contentMode, currentFile, currentSlideIndex, editorBaseMarkdown, editorFilePath, editorMode, editorSlideIndex, editorText, isEditorOpen, rawMarkdown]);

  const restoreAIRewriteVersion = useCallback((source: string, label: string) => {
    if (!source.trim()) return;
    setEditorText(source.trim());
    setEditorError(`已恢复到${label}。这只是更新左侧草稿，确认后请点击保存。`);
    setIsEditorOpen(true);
  }, []);

  const saveCurrentSlide = useCallback(async () => {
    const targetFile = editorFilePath || currentFile;
    if (contentMode || !targetFile) return;
    const sourceMarkdown = editorBaseMarkdown || rawMarkdown;
    const doc = parseEditableSlideDocument(sourceMarkdown);
    const index = editorSlideIndex ?? currentSlideIndex;
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
      await saveEditableDocument(doc, index, targetFile);
      closeEditor();
    } catch (e: any) {
      setEditorError(String(e?.message || e));
    } finally {
      setIsSaving(false);
    }
  }, [closeEditor, contentMode, currentFile, currentSlideIndex, editorBaseMarkdown, editorFilePath, editorMode, editorSlideIndex, editorText, rawMarkdown, saveEditableDocument]);

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
      <style>{CORE_LAYOUT_CSS}</style>
      <style>{tpl.css}</style>
      <div
        className="slide-stage"
        style={{ transform: `scale(${stageScale})` }}
        aria-label="16:9 presentation stage"
      >
        <div className={`reveal theme-${tpl.revealTheme}`} ref={containerRef} onClick={handleStageClick}>
          <div className="slides">
            {slides.map((slide) => (
              <section key={slide.index} {...(slide.revealAttrs || {})} className={slide.class || ''}>
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
          <div className="slide-overview-list" ref={overviewListRef}>
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
                  data-slide-index={slide.index}
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
                        <section {...(slide.revealAttrs || {})} className={`${slide.class || ''} present`.trim()}>
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

      {actionNotice && (
        <div className={`slide-action-toast ${actionNotice.kind}`} role="status">
          <span>{actionNotice.text}</span>
          <button onClick={() => setActionNotice(null)} title="Close">×</button>
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
          <div className={`slide-edit-dialog ${isAIModalOpen ? 'with-ai' : ''}`}>
            <div className="slide-edit-header">
              <div>
                <div className="slide-edit-title">{editorMode === 'insert' ? `插入第 ${editorDisplayIndex + 1} 页` : `编辑第 ${editorDisplayIndex + 1} 页`}</div>
                <div className="slide-edit-subtitle">{editorMode === 'insert' ? `${editorFilePath || currentFile} · 新页面将插入到当前位置` : (editorFilePath || currentFile)}</div>
              </div>
              <button className="slide-edit-close" onClick={closeEditor} title="Close">×</button>
            </div>
            <div className="slide-edit-workspace">
              <textarea
                className="slide-edit-textarea"
                value={editorText}
                onChange={(event) => setEditorText(event.target.value)}
                spellCheck={false}
                autoFocus
              />
              {isAIModalOpen && (
                <aside className="slide-ai-panel" aria-label="AI iterative rewrite panel">
                  <div className="slide-ai-panel-header">
                    <div>
                      <div className="slide-ai-panel-title">AI 改稿会话</div>
                      <div className="slide-ai-panel-subtitle">每一轮都基于左侧最新草稿，满意后再保存。</div>
                    </div>
                    <button className="slide-ai-panel-close" onClick={() => setIsAIModalOpen(false)} title="收起 AI 改稿">×</button>
                  </div>
                  <div className="slide-ai-history">
                    {aiRewriteTurns.length === 0 ? (
                      <div className="slide-ai-empty">
                        输入本轮修改意见。AI 会改写左侧草稿，但不会自动保存到文件。
                      </div>
                    ) : aiRewriteTurns.map((turn, index) => (
                      <div className="slide-ai-turn" key={`${index}-${turn.instruction}`}>
                        <div className="slide-ai-turn-user"><strong>你：</strong>{turn.instruction}</div>
                        <div className="slide-ai-turn-assistant"><strong>AI：</strong>{turn.summary}</div>
                        <div className="slide-ai-turn-actions">
                          <button
                            type="button"
                            onClick={() => restoreAIRewriteVersion(turn.before, `第 ${index + 1} 轮修改前`)}
                            disabled={isAIWorking}
                            title="把左侧草稿恢复为本轮 AI 修改前的内容"
                          >
                            回退到修改前
                          </button>
                          <button
                            type="button"
                            onClick={() => restoreAIRewriteVersion(turn.after, `第 ${index + 1} 轮 AI 结果`)}
                            disabled={isAIWorking}
                            title="把左侧草稿恢复为本轮 AI 生成后的内容"
                          >
                            恢复 AI 结果
                          </button>
                        </div>
                      </div>
                    ))}
                  </div>
                  <div className="slide-ai-quick-actions">
                    {['再短一点', '更图像化', '结构不对，重新组织', '语气更自然', '变化还不够明显'].map(text => (
                      <button
                        key={text}
                        type="button"
                        onClick={() => setAIInstruction(prev => prev ? `${prev}\n${text}` : text)}
                        disabled={isAIWorking}
                      >
                        {text}
                      </button>
                    ))}
                  </div>
                  <textarea
                    className="slide-ai-textarea"
                    value={aiInstruction}
                    onChange={(event) => setAIInstruction(event.target.value)}
                    placeholder="例如：保留现在的布局，但把文字减少 30%，右侧图再强化一点。"
                    disabled={isAIWorking}
                  />
                  {aiError && <div className="slide-edit-error slide-ai-error">{aiError}</div>}
                  <div className="slide-ai-actions">
                    <button className="slide-edit-secondary" onClick={() => setAIInstruction('')} disabled={isAIWorking || !aiInstruction.trim()}>清空意见</button>
                    <button className="slide-edit-primary" onClick={submitAIRewrite} disabled={isAIWorking || !aiInstruction.trim()}>
                      {isAIWorking ? '改稿中…' : aiRewriteTurns.length > 0 ? '继续修改' : '生成修改稿'}
                    </button>
                  </div>
                </aside>
              )}
            </div>
            {editorError && <div className="slide-edit-error">{editorError}</div>}
            <div className="slide-edit-actions">
              <button className="slide-edit-secondary" onClick={openAIRewriteFromEditor} disabled={isSaving || isAIWorking}>{isAIModalOpen ? '💬 AI 面板已打开' : '💬 AI 修改'}</button>
              <button className="slide-edit-secondary" onClick={closeEditor} disabled={isSaving}>取消</button>
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
    </div>
  );
}
