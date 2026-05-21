import {
  forwardRef,
  useEffect,
  useImperativeHandle,
  useRef,
  useState,
} from "react";
import {
  DocumentLoader,
  type BookDoc,
  type TOCItem,
} from "@/reader/readest/libs/document";
import { ContinuumReaderService } from "@/reader/continuum/ContinuumReaderService";
import type { Annotation } from "@/lib/api";

type FoliateViewElement = HTMLElement & {
  open: (book: BookDoc) => Promise<void>;
  close?: () => void;
  init: (options: { lastLocation: string }) => Promise<void>;
  goToFraction: (fraction: number) => Promise<void>;
  goTo: (href: string) => void;
  getCFI: (index: number, range: Range) => string;
  addAnnotation: (
    annotation: {
      value: string;
      color?: string;
      style?: string;
      note?: string;
    },
    remove?: boolean,
  ) => void;
  deselect: () => void;
  search: (opts: {
    index?: number;
    matchCase?: boolean;
    matchDiacritics?: boolean;
    matchWholeWords?: boolean;
    query: string;
  }) => AsyncGenerator<
    | string
    | {
        cfi?: string;
        excerpt?: SearchExcerpt;
        label?: string;
        subitems?: Array<{ cfi: string; excerpt?: SearchExcerpt }>;
      },
    void,
    void
  >;
  clearSearch: () => void;
  renderer?: {
    primaryIndex?: number;
    getContents: () => Array<{ doc: Document; index?: number }>;
  };
  next?: () => void;
  prev?: () => void;
};

type RelocateDetail = {
  cfi?: string;
  location?: {
    current?: number;
    total?: number;
  };
  tocItem?: {
    label?: string;
  };
  section?: {
    label?: string;
    title?: string;
  };
  title?: string;
};

type Props = {
  annotations?: Annotation[];
  bookID: string;
  format: string;
  /**
   * Portal-signed URL for the book's file bytes. When set, the reader
   * fetches via this URL (no Authorization header needed; signed ?token=
   * carries auth). When undefined, falls back to the portal proxy endpoint.
   */
  fileUrl?: string;
  settings?: {
    flow?: "paginated" | "scrolled";
    fontFamily?: string;
    fontSize?: number;
    fontWeight?: number;
    hyphenation?: boolean;
    lineHeight?: number;
    margin?: number;
    maxWidth?: number;
    paragraphFocus?: boolean;
    rtl?: boolean;
    spread?: "auto" | "none";
    theme?: "light" | "sepia" | "dark";
    writingMode?: "auto" | "horizontal-tb" | "vertical-rl";
    zoom?: number;
  };
  onContentPopup?: (popup: ReaderContentPopup | null) => void;
  onDiagnostic?: (entry: ReaderDiagnostic) => void;
  onReady?: (data: { readaloud: boolean; toc: TOCItem[] }) => void;
  onProgress?: (progress: { cfi: string; percentage: number }) => void;
  onSectionChange?: (section: string) => void;
  onSelectionChange?: (selection: ReaderSelection | null) => void;
};

export type SearchExcerpt = {
  pre?: string;
  match?: string;
  post?: string;
};

export type ReaderSearchResult = {
  cfi: string;
  label?: string;
  excerpt?: SearchExcerpt;
};

export type ReaderSearchOptions = {
  matchCase?: boolean;
  matchDiacritics?: boolean;
  matchWholeWords?: boolean;
  scope?: "book" | "section";
};

export type ReaderContentPopup = {
  kind: "footnote" | "image" | "table";
  title: string;
  content?: string;
  html?: string;
  src?: string;
};

export type ReaderSelection = {
  cfi: string;
  rect: {
    height: number;
    left: number;
    top: number;
    width: number;
  };
  selectedText: string;
};

export type ReaderDiagnostic = {
  at: number;
  level: "info" | "warn" | "error";
  message: string;
};

export type ReadestLiteReaderHandle = {
  clearSearch: () => void;
  clearSelection: () => void;
  flushProgress: () => Promise<void>;
  createSelectionAnnotation: () => ReaderSelection | null;
  getReadableText: () => string;
  goToFraction: (fraction: number) => Promise<void>;
  goTo: (href: string) => void;
  next: () => void;
  prev: () => void;
  search: (
    query: string,
    options?: ReaderSearchOptions,
  ) => Promise<ReaderSearchResult[]>;
};

export const ReadestLiteReader = forwardRef<ReadestLiteReaderHandle, Props>(
  function ReadestLiteReader(
    {
      annotations = [],
      bookID,
      format,
      fileUrl,
      onContentPopup,
      onDiagnostic,
      onReady,
      onProgress,
      onSectionChange,
      onSelectionChange,
      settings,
    },
    ref,
  ) {
    const containerRef = useRef<HTMLDivElement>(null);
    const viewRef = useRef<FoliateViewElement | null>(null);
    const initializedRef = useRef(false);
    const configRef = useRef<Record<string, unknown>>({});
    const pendingConfigRef = useRef<Record<string, unknown> | null>(null);
    const progressTimerRef = useRef<number | null>(null);
    const serviceRef = useRef<ContinuumReaderService | null>(null);
    const annotationsRef = useRef<Annotation[]>(annotations);
    const drawnCfisRef = useRef<Set<string>>(new Set());
    const paragraphFocusCleanupRef = useRef<(() => void)[]>([]);
    const selectionCleanupRef = useRef<(() => void)[]>([]);
    const [error, setError] = useState("");
    const [loading, setLoading] = useState(true);

    const emitDiagnostic = (
      level: ReaderDiagnostic["level"],
      message: string,
    ) => {
      onDiagnostic?.({ at: Date.now(), level, message });
    };

    const flushProgress = async () => {
      if (progressTimerRef.current !== null) {
        window.clearTimeout(progressTimerRef.current);
        progressTimerRef.current = null;
      }
      const pending = pendingConfigRef.current;
      const service = serviceRef.current;
      if (!pending || !service) return;
      pendingConfigRef.current = null;
      await service.saveBookConfig(bookID, pending);
      emitDiagnostic("info", "Progress saved");
    };

    const scheduleProgressSave = (config: Record<string, unknown>) => {
      pendingConfigRef.current = config;
      if (progressTimerRef.current !== null) {
        window.clearTimeout(progressTimerRef.current);
      }
      progressTimerRef.current = window.setTimeout(() => {
        void flushProgress().catch((err) => {
          emitDiagnostic(
            "error",
            err instanceof Error ? err.message : "Unable to save progress",
          );
        });
      }, 1200);
    };

    const readerStyles = () => {
      const fontSize = settings?.fontSize ?? 100;
      const fontFamily = settings?.fontFamily ?? "inherit";
      const fontWeight = settings?.fontWeight ?? 400;
      const hyphenation = settings?.hyphenation ?? true;
      const lineHeight = settings?.lineHeight ?? 1.6;
      const maxWidth = settings?.maxWidth ?? 72;
      const paragraphFocus = settings?.paragraphFocus ?? false;
      const rtl = settings?.rtl ?? false;
      const writingMode = settings?.writingMode ?? "auto";
      const theme = settings?.theme ?? "light";
      const palette =
        theme === "dark"
          ? { bg: "#111111", fg: "#f5f5f5", link: "#93c5fd" }
          : theme === "sepia"
            ? { bg: "#f4ecd8", fg: "#1f1b16", link: "#8a4b12" }
            : { bg: "#ffffff", fg: "#171717", link: "#2563eb" };
      return `
        html, body {
          background: ${palette.bg} !important;
          color: ${palette.fg} !important;
          direction: ${rtl ? "rtl" : "inherit"} !important;
          font-family: ${fontFamily} !important;
          font-size: ${fontSize}% !important;
          font-weight: ${fontWeight} !important;
          hyphens: ${hyphenation ? "auto" : "none"} !important;
          line-height: ${lineHeight} !important;
          max-width: ${maxWidth}ch !important;
          writing-mode: ${writingMode === "auto" ? "inherit" : writingMode} !important;
        }
        p, li, blockquote { margin-block: 0.75em !important; }
        ${
          paragraphFocus
            ? `body :is(p, li, blockquote, h1, h2, h3, h4, h5, h6) {
                 opacity: 0.28;
                 transition: opacity 120ms ease, filter 120ms ease;
               }
               body :is(p, li, blockquote, h1, h2, h3, h4, h5, h6)[data-continuum-current-paragraph="true"] {
                 opacity: 1;
                 filter: none;
               }`
            : ""
        }
        a { color: ${palette.link} !important; }
      `;
    };

    const applyReaderSettings = () => {
      const view = viewRef.current;
      const renderer = view?.renderer as
        | (HTMLElement & {
            setStyles?: (css: string) => void;
            render?: () => Promise<void>;
          })
        | undefined;
      if (!renderer) return;
      renderer.setStyles?.(readerStyles());
      if (settings?.flow === "scrolled") {
        renderer.setAttribute("flow", "scrolled");
      } else {
        renderer.removeAttribute("flow");
      }
      renderer.setAttribute("gap", `${Math.max(0, settings?.margin ?? 24)}px`);
      renderer.setAttribute(
        "margin",
        `${Math.max(0, settings?.margin ?? 24)}px`,
      );
      renderer.setAttribute("max-inline-size", `${settings?.maxWidth ?? 72}ch`);
      renderer.setAttribute("scale", `${settings?.zoom ?? 100}`);
      renderer.setAttribute("spread", settings?.spread ?? "auto");
      void renderer.render?.();
    };

    const drawAnnotation = (annotation: Annotation) => {
      const view = viewRef.current;
      if (!view || !annotation.cfi_range || annotation.deleted_at) return;
      if (
        annotation.kind === "bookmark" ||
        annotation.readest_type === "bookmark"
      ) {
        return;
      }
      view.addAnnotation({
        value: annotation.cfi_range,
        color: annotation.color || "#facc15",
        style: annotation.style || "highlight",
        note: annotation.note_text,
      });
      drawnCfisRef.current.add(annotation.cfi_range);
    };

    const drawAnnotations = () => {
      const view = viewRef.current;
      if (!view) return;
      const currentAnnotations = annotationsRef.current;
      const activeCfis = new Set(
        currentAnnotations
          .filter(
            (annotation) =>
              annotation.cfi_range &&
              !annotation.deleted_at &&
              annotation.kind !== "bookmark" &&
              annotation.readest_type !== "bookmark",
          )
          .map((annotation) => annotation.cfi_range),
      );
      for (const cfi of drawnCfisRef.current) {
        if (!activeCfis.has(cfi)) {
          view.addAnnotation({ value: cfi }, true);
          drawnCfisRef.current.delete(cfi);
        }
      }
      for (const annotation of currentAnnotations) drawAnnotation(annotation);
    };

    const createSelectionAnnotation = (): ReaderSelection | null => {
      const view = viewRef.current;
      if (!view) return null;
      const contents = view?.renderer?.getContents?.() ?? [];
      for (const content of contents) {
        const selection = content.doc.getSelection();
        if (!selection || selection.isCollapsed || selection.rangeCount === 0) {
          continue;
        }
        const range = selection.getRangeAt(0);
        const selectedText = selection.toString().trim();
        if (!selectedText) continue;
        const cfi = view.getCFI(content.index ?? 0, range);
        const rangeRect = range.getBoundingClientRect();
        const frameRect =
          content.doc.defaultView?.frameElement?.getBoundingClientRect();
        return {
          cfi,
          rect: {
            height: rangeRect.height,
            left: rangeRect.left + (frameRect?.left ?? 0),
            top: rangeRect.top + (frameRect?.top ?? 0),
            width: rangeRect.width,
          },
          selectedText,
        };
      }
      return null;
    };

    const emitSelectionChange = () => {
      onSelectionChange?.(createSelectionAnnotation());
    };

    const clearParagraphFocus = () => {
      const contents = viewRef.current?.renderer?.getContents?.() ?? [];
      for (const content of contents) {
        content.doc
          .querySelectorAll("[data-continuum-current-paragraph]")
          .forEach((element) =>
            element.removeAttribute("data-continuum-current-paragraph"),
          );
      }
    };

    const updateParagraphFocus = () => {
      if (!settings?.paragraphFocus) {
        clearParagraphFocus();
        return;
      }
      const contents = viewRef.current?.renderer?.getContents?.() ?? [];
      for (const content of contents) {
        const doc = content.doc;
        const candidates = Array.from(
          doc.querySelectorAll<HTMLElement>(
            "p, li, blockquote, h1, h2, h3, h4, h5, h6",
          ),
        ).filter((element) => element.textContent?.trim());
        let nearest: HTMLElement | null = null;
        let nearestDistance = Number.POSITIVE_INFINITY;
        const midpoint =
          (doc.defaultView?.innerHeight ?? window.innerHeight) / 2;
        for (const candidate of candidates) {
          const rect = candidate.getBoundingClientRect();
          if (
            rect.bottom < 0 ||
            rect.top > (doc.defaultView?.innerHeight ?? 0)
          ) {
            continue;
          }
          const distance = Math.abs(rect.top + rect.height / 2 - midpoint);
          if (distance < nearestDistance) {
            nearest = candidate;
            nearestDistance = distance;
          }
        }
        for (const candidate of candidates) {
          if (candidate === nearest) {
            candidate.setAttribute("data-continuum-current-paragraph", "true");
          } else {
            candidate.removeAttribute("data-continuum-current-paragraph");
          }
        }
      }
    };

    const attachParagraphFocusListeners = () => {
      for (const cleanup of paragraphFocusCleanupRef.current) cleanup();
      paragraphFocusCleanupRef.current = [];
      const contents = viewRef.current?.renderer?.getContents?.() ?? [];
      for (const content of contents) {
        const doc = content.doc;
        const target = doc.defaultView ?? doc;
        const handler = () =>
          window.requestAnimationFrame(updateParagraphFocus);
        target.addEventListener("scroll", handler);
        doc.addEventListener("pointermove", handler);
        doc.addEventListener("keyup", handler);
        paragraphFocusCleanupRef.current.push(() => {
          target.removeEventListener("scroll", handler);
          doc.removeEventListener("pointermove", handler);
          doc.removeEventListener("keyup", handler);
        });
      }
      updateParagraphFocus();
    };

    const attachSelectionListeners = () => {
      for (const cleanup of selectionCleanupRef.current) cleanup();
      selectionCleanupRef.current = [];

      const contents = viewRef.current?.renderer?.getContents?.() ?? [];
      for (const content of contents) {
        const doc = content.doc;
        const handler = () => {
          window.setTimeout(emitSelectionChange, 0);
        };
        doc.addEventListener("selectionchange", handler);
        doc.addEventListener("pointerup", handler);
        doc.addEventListener("keyup", handler);
        const clickHandler = (event: MouseEvent) => {
          const target = event.target as Element | null;
          const image = target?.closest("img") as HTMLImageElement | null;
          if (image?.src) {
            event.preventDefault();
            onContentPopup?.({
              kind: "image",
              src: image.src,
              title: image.alt || "Image",
            });
            return;
          }

          const table = target?.closest("table") as HTMLTableElement | null;
          if (table) {
            event.preventDefault();
            onContentPopup?.({
              kind: "table",
              html: table.outerHTML,
              title: "Table",
            });
            return;
          }

          const anchor = target?.closest("a[href]") as HTMLAnchorElement | null;
          const href = anchor?.getAttribute("href") ?? "";
          if (!anchor || !href.startsWith("#")) return;
          const footnoteTarget = doc.getElementById(
            decodeURIComponent(href.slice(1)),
          );
          if (!footnoteTarget) return;
          event.preventDefault();
          onContentPopup?.({
            kind: "footnote",
            content: footnoteTarget.textContent?.replace(/\s+/g, " ").trim(),
            title: anchor.textContent?.trim() || "Footnote",
          });
        };
        doc.addEventListener("click", clickHandler);
        selectionCleanupRef.current.push(() => {
          doc.removeEventListener("selectionchange", handler);
          doc.removeEventListener("pointerup", handler);
          doc.removeEventListener("keyup", handler);
          doc.removeEventListener("click", clickHandler);
        });
      }
    };

    const getReadableText = () => {
      const contents = viewRef.current?.renderer?.getContents?.() ?? [];
      for (const content of contents) {
        const selection = content.doc.getSelection();
        const selectedText = selection?.toString().trim();
        if (selectedText) return selectedText;
      }
      const primaryIndex = viewRef.current?.renderer?.primaryIndex;
      const primary =
        contents.find((content) => content.index === primaryIndex) ??
        contents[0];
      return (primary?.doc.body?.innerText ?? "")
        .replace(/\s+/g, " ")
        .trim()
        .slice(0, 5000);
    };

    useImperativeHandle(ref, () => ({
      clearSearch: () => viewRef.current?.clearSearch?.(),
      clearSelection: () => {
        viewRef.current?.deselect?.();
        onSelectionChange?.(null);
      },
      flushProgress,
      createSelectionAnnotation,
      getReadableText,
      goToFraction: (fraction: number) =>
        viewRef.current?.goToFraction(fraction) ?? Promise.resolve(),
      goTo: (href: string) => viewRef.current?.goTo(href),
      next: () => viewRef.current?.next?.(),
      prev: () => viewRef.current?.prev?.(),
      search: async (query: string, options?: ReaderSearchOptions) => {
        const view = viewRef.current;
        if (!view || !query.trim()) return [];
        const index =
          options?.scope === "section"
            ? view.renderer?.primaryIndex
            : undefined;
        const results: ReaderSearchResult[] = [];
        for await (const result of view.search({
          index,
          matchCase: options?.matchCase,
          matchDiacritics: options?.matchDiacritics,
          matchWholeWords: options?.matchWholeWords,
          query: query.trim(),
        })) {
          if (typeof result === "string") continue;
          if (result.cfi) {
            results.push({
              cfi: result.cfi,
              excerpt: result.excerpt,
            });
          }
          for (const subitem of result.subitems ?? []) {
            results.push({
              cfi: subitem.cfi,
              label: result.label,
              excerpt: subitem.excerpt,
            });
          }
        }
        return results;
      },
    }));

    useEffect(() => {
      let cancelled = false;
      const service = new ContinuumReaderService();
      serviceRef.current = service;
      initializedRef.current = false;
      setError("");
      setLoading(true);

      async function open() {
        try {
          const [file, config] = await Promise.all([
            service.loadBookContent(bookID, format, fileUrl),
            service.loadBookConfig(bookID),
          ]);
          emitDiagnostic("info", `Loaded ${format.toUpperCase()} source`);
          configRef.current = config;
          const { book } = await new DocumentLoader(file).open();
          if (cancelled) return;

          await import("foliate-js/view.js");
          const view = document.createElement(
            "foliate-view",
          ) as FoliateViewElement;
          viewRef.current = view;
          containerRef.current?.replaceChildren(view);

          view.addEventListener("draw-annotation", async (event: Event) => {
            const { Overlayer } = await import("foliate-js/overlayer.js");
            const detail = (
              event as CustomEvent<{
                annotation: { color?: string; style?: string };
                draw: (fn: unknown, options?: Record<string, unknown>) => void;
              }>
            ).detail;
            const style = detail.annotation.style || "highlight";
            const color = detail.annotation.color || "#facc15";
            const draw =
              style === "underline"
                ? Overlayer.underline
                : style === "squiggly"
                  ? Overlayer.squiggly
                  : Overlayer.highlight;
            detail.draw(draw, { color });
          });

          view.addEventListener("create-overlay", () => {
            attachSelectionListeners();
            attachParagraphFocusListeners();
            drawAnnotations();
          });

          view.addEventListener("relocate", (event: Event) => {
            if (!initializedRef.current) return;
            const detail = (event as CustomEvent<RelocateDetail>).detail;
            const location = detail?.cfi;
            const current = detail?.location?.current ?? 0;
            const total = detail?.location?.total ?? 0;
            if (!location || total <= 0) return;
            const section =
              detail?.tocItem?.label ||
              detail?.section?.label ||
              detail?.section?.title ||
              detail?.title ||
              "";
            if (section) onSectionChange?.(section);
            const nextConfig = {
              ...configRef.current,
              location,
              progress: [current + 1, total],
              updatedAt: Date.now(),
            };
            configRef.current = nextConfig;
            onProgress?.({ cfi: location, percentage: (current + 1) / total });
            updateParagraphFocus();
            scheduleProgressSave(nextConfig);
          });

          await view.open(book);
          emitDiagnostic("info", "Reader opened");
          applyReaderSettings();
          const bookWithCapabilities = book as BookDoc & {
            mediaOverlay?: unknown;
            mediaOverlays?: unknown;
            media_overlay?: unknown;
          };
          onReady?.({
            readaloud: Boolean(
              bookWithCapabilities.mediaOverlay ||
              bookWithCapabilities.mediaOverlays ||
              bookWithCapabilities.media_overlay,
            ),
            toc: book.toc ?? [],
          });
          attachSelectionListeners();
          attachParagraphFocusListeners();
          drawAnnotations();
          const lastLocation =
            typeof config.location === "string" && config.location.length > 0
              ? config.location
              : undefined;
          if (lastLocation) {
            await view.init({ lastLocation });
          } else {
            await view.goToFraction(0);
          }
          initializedRef.current = true;
          setLoading(false);
        } catch (err) {
          if (!cancelled) {
            const message =
              err instanceof Error ? err.message : "Unable to open book";
            emitDiagnostic("error", message);
            setError(message);
            setLoading(false);
          }
        }
      }

      void open();
      const flush = () => {
        void flushProgress();
      };
      document.addEventListener("visibilitychange", flush);
      window.addEventListener("pagehide", flush);
      window.addEventListener("beforeunload", flush);
      return () => {
        cancelled = true;
        void flushProgress();
        document.removeEventListener("visibilitychange", flush);
        window.removeEventListener("pagehide", flush);
        window.removeEventListener("beforeunload", flush);
        initializedRef.current = false;
        viewRef.current?.close?.();
        viewRef.current?.remove();
        viewRef.current = null;
        serviceRef.current = null;
        drawnCfisRef.current.clear();
        for (const cleanup of paragraphFocusCleanupRef.current) cleanup();
        paragraphFocusCleanupRef.current = [];
        for (const cleanup of selectionCleanupRef.current) cleanup();
        selectionCleanupRef.current = [];
      };
    }, [
      bookID,
      format,
      onContentPopup,
      onDiagnostic,
      onReady,
      onProgress,
      onSectionChange,
      onSelectionChange,
    ]);

    useEffect(() => {
      annotationsRef.current = annotations;
      drawAnnotations();
    }, [annotations]);

    useEffect(() => {
      applyReaderSettings();
      attachParagraphFocusListeners();
    }, [
      settings?.flow,
      settings?.fontFamily,
      settings?.fontSize,
      settings?.fontWeight,
      settings?.hyphenation,
      settings?.lineHeight,
      settings?.margin,
      settings?.maxWidth,
      settings?.paragraphFocus,
      settings?.rtl,
      settings?.spread,
      settings?.theme,
      settings?.writingMode,
      settings?.zoom,
    ]);

    return (
      <div className="relative h-full w-full overflow-hidden bg-background">
        <div ref={containerRef} className="h-full w-full" />
        {loading && !error ? (
          <div className="absolute inset-0 flex items-center justify-center bg-background text-sm text-muted-foreground">
            Loading reader...
          </div>
        ) : null}
        {error ? (
          <div className="absolute inset-0 flex items-center justify-center p-6 text-center text-sm text-destructive">
            {error}
          </div>
        ) : null}
      </div>
    );
  },
);
