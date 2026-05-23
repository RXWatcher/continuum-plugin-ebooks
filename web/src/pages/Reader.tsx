import { useCallback, useEffect, useRef, useState } from "react";
import { useScreenWakeLock } from "@/hooks/useScreenWakeLock";
import { useEinkMode } from "@/hooks/useEinkMode";
import { Link, useParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  ArrowLeft,
  ArrowRight,
  Bookmark,
  ChevronLeft,
  ChevronDown,
  ChevronUp,
  Columns2,
  Copy,
  Download,
  BookOpen,
  Highlighter,
  Languages,
  ListTree,
  Maximize2,
  NotebookPen,
  Pause,
  Pencil,
  Play,
  Ruler,
  Search,
  Settings,
  SkipBack,
  SkipForward,
  Square,
  Sun,
  Timer,
  Trash2,
  Type,
  Volume2,
} from "lucide-react";
import {
  createAnnotation,
  deleteAnnotation,
  getBook,
  getReaderConfig,
  linkKosyncBook,
  listAnnotations,
  listCustomFonts,
  mountPath,
  putReaderConfig,
  updateAnnotation,
  type Annotation,
  type ExternalReaderProgress,
} from "@/lib/api";
import CustomFontUploader from "@/components/CustomFontUploader";
import DefinePopover from "@/components/DefinePopover";
import TranslatePopover from "@/components/TranslatePopover";
import { Button } from "@/components/ui/button";
import {
  ReadestLiteReader,
  type ReaderContentPopup,
  type ReaderDiagnostic,
  type ReadestLiteReaderHandle,
  type ReaderSearchOptions,
  type ReaderSearchResult,
  type ReaderSelection,
} from "@/reader/ReadestLiteReader";
import type { TOCItem } from "@/reader/readest/libs/document";
import { CFI } from "@/reader/readest/libs/document";

type ReaderPanelTab = "toc" | "search" | "notes" | "bookmarks" | "settings";
type HighlightStyle = "highlight" | "underline" | "squiggly";
type ReaderTheme = "light" | "sepia" | "dark";
type ReaderFlow = "paginated" | "scrolled";
type WritingMode = "auto" | "horizontal-tb" | "vertical-rl";
type QuickAction = "highlight" | "note" | "search" | "speak";

// Softer highlight palette than the default tailwind 400 family — those
// shades are too saturated on a white page and harsh against dark themes.
// These are tailwind 300/200-equivalent: still legible, much friendlier.
const highlightColors = [
  { label: "Yellow", value: "#f4d03f" },
  { label: "Coral", value: "#fca5a5" },
  { label: "Mint", value: "#a7f3d0" },
  { label: "Sky", value: "#bae6fd" },
  { label: "Lavender", value: "#ddd6fe" },
];

// Reader presets give users a one-click way to set sensible typography
// without dragging four sliders. Each preset bumps fontFamily, lineHeight,
// and (where it matters) margins. Picked from typical "long-form reading"
// recommendations: 1.7–1.9 line-height + generous margins reduce eye strain.
type ReaderPreset = {
  id: 'comfortable' | 'accessible' | 'compact';
  label: string;
  description: string;
  fontFamily: string;
  fontSize: number;
  lineHeight: number;
};
const readerPresets: ReaderPreset[] = [
  {
    id: 'comfortable',
    label: 'Comfortable',
    description: 'Serif, generous spacing — good for novels',
    fontFamily: 'Merriweather, Georgia, serif',
    fontSize: 110,
    lineHeight: 1.8,
  },
  {
    id: 'accessible',
    label: 'Accessible',
    description: 'Larger text, looser leading, sans-serif',
    fontFamily: 'system-ui, sans-serif',
    fontSize: 125,
    lineHeight: 1.95,
  },
  {
    id: 'compact',
    label: 'Compact',
    description: 'Tighter — fits more text per page',
    fontFamily: 'serif',
    fontSize: 95,
    lineHeight: 1.55,
  },
];

const highlightStyles: Array<{ label: string; value: HighlightStyle }> = [
  { label: "Highlight", value: "highlight" },
  { label: "Underline", value: "underline" },
  { label: "Squiggle", value: "squiggly" },
];

const globalReaderDefaultsKey = "silo-ebooks-reader-defaults";

// clipExcerpt trims foliate-js search-result excerpt segments so cards stay
// scannable. "lead" trims from the front (showing the tail of pre-match
// context near the highlight); "trail" trims from the end. Single-purpose
// helper keeps the JSX above readable.
function clipExcerpt(s: string | undefined | null, max: number, side: "lead" | "trail"): string {
  if (!s) return "";
  if (s.length <= max) return s;
  return side === "lead" ? "…" + s.slice(s.length - max) : s.slice(0, max) + "…";
}

export default function Reader() {
  // Screen wake-lock — keeps the device screen on while the reader
  // page is mounted. Hook silently no-ops on browsers without the
  // Wake Lock API. Released automatically on unmount.
  useScreenWakeLock(true);
  // E-ink mode persists in localStorage; the hook installs the body
  // class so the global stylesheet can disable animations / blurs.
  // We don't expose the toggle in the reader chrome yet — users
  // bootstrap by setting localStorage.ebooks.eink.enabled = "true"
  // (or via the settings panel once we wire one).
  useEinkMode();

  const params = useParams();
  const id = params.id ?? "";
  const readerRef = useRef<ReadestLiteReaderHandle | null>(null);
  const [pct, setPct] = useState(0);
  const [scrubPct, setScrubPct] = useState(0);
  const [currentSection, setCurrentSection] = useState("");
  // New default matches the "Comfortable" preset — better long-form reading
  // out of the box. Persisted preferences override these on mount.
  const [fontSize, setFontSize] = useState(110);
  const [theme, setTheme] = useState<ReaderTheme>("light");
  const [spread, setSpread] = useState<"auto" | "none">("auto");
  const [flow, setFlow] = useState<ReaderFlow>("paginated");
  const [fontFamily, setFontFamily] = useState("Merriweather, Georgia, serif");
  const [fontWeight, setFontWeight] = useState(400);
  const [hyphenation, setHyphenation] = useState(true);
  const [lineHeight, setLineHeight] = useState(1.8);
  const [margin, setMargin] = useState(24);
  const [maxWidth, setMaxWidth] = useState(72);
  const [fontBrightness, setFontBrightness] = useState(100);
  const [readingRuler, setReadingRuler] = useState(false);
  const [rulerTop, setRulerTop] = useState(50);
  const rulerDragRef = useRef<{ offsetY: number } | null>(null);
  const [rtl, setRtl] = useState(false);
  const [writingMode, setWritingMode] = useState<WritingMode>("auto");
  const [zoom, setZoom] = useState(100);
  const [selectedFormat, setSelectedFormat] = useState("epub");
  const [currentCfi, setCurrentCfi] = useState("");
  const [panelTab, setPanelTab] = useState<ReaderPanelTab>("toc");
  const [showPanel, setShowPanel] = useState(true);
  const [toc, setToc] = useState<TOCItem[]>([]);
  const [readaloudAvailable, setReadaloudAvailable] = useState(false);
  const [noteText, setNoteText] = useState("");
  const [editingAnnotation, setEditingAnnotation] = useState<Annotation | null>(
    null,
  );
  const [pendingNoteSelection, setPendingNoteSelection] =
    useState<ReaderSelection | null>(null);
  const [readerSelection, setReaderSelection] =
    useState<ReaderSelection | null>(null);
  const [highlightStyle, setHighlightStyle] =
    useState<HighlightStyle>("highlight");
  const [highlightColor, setHighlightColor] = useState("#f4d03f");
  const [customHighlightColors, setCustomHighlightColors] = useState<string[]>(
    [],
  );
  const [quickAction, setQuickAction] = useState<QuickAction>("highlight");
  const [searchTerm, setSearchTerm] = useState("");
  const [searchOptions, setSearchOptions] = useState<
    Required<ReaderSearchOptions>
  >({
    matchCase: false,
    matchDiacritics: false,
    matchWholeWords: false,
    scope: "book",
  });
  const [searching, setSearching] = useState(false);
  const [searchResults, setSearchResults] = useState<ReaderSearchResult[]>([]);
  const [searchResultIndex, setSearchResultIndex] = useState(0);
  const [ttsActive, setTtsActive] = useState(false);
  const [ttsPaused, setTtsPaused] = useState(false);
  const [ttsRate, setTtsRate] = useState(1);
  const [ttsVoice, setTtsVoice] = useState("");
  const [ttsVoices, setTtsVoices] = useState<SpeechSynthesisVoice[]>([]);
  const [ttsChunks, setTtsChunks] = useState<string[]>([]);
  const [ttsChunkIndex, setTtsChunkIndex] = useState(0);
  const [ttsSleepMinutes, setTtsSleepMinutes] = useState(0);
  const [rsvpActive, setRsvpActive] = useState(false);
  const [rsvpWpm, setRsvpWpm] = useState(300);
  const [rsvpWords, setRsvpWords] = useState<string[]>([]);
  const [rsvpIndex, setRsvpIndex] = useState(0);
  const [noteSearchTerm, setNoteSearchTerm] = useState("");
  const [contentPopup, setContentPopup] = useState<ReaderContentPopup | null>(
    null,
  );
  const [diagnostics, setDiagnostics] = useState<ReaderDiagnostic[]>([]);
  const [externalProgress, setExternalProgress] =
    useState<ExternalReaderProgress | null>(null);
  const [kosyncDocument, setKosyncDocument] = useState("");
  const [settingsLoaded, setSettingsLoaded] = useState(false);

  const detail = useQuery({
    queryKey: ["book", id],
    queryFn: () => getBook(id),
    enabled: !!id,
  });
  const annotations = useQuery({
    queryKey: ["annotations", id],
    queryFn: () => listAnnotations(id),
    enabled: !!id,
  });

  // Define / Translate popovers. Each holds the selected text +
  // a position; the popover component does its own data fetch so
  // a slow lookup doesn't block the reader UI.
  const [definePopover, setDefinePopover] = useState<{ word: string } | null>(
    null,
  );
  const [translatePopover, setTranslatePopover] = useState<{ text: string } | null>(
    null,
  );

  const defineSelection = () => {
    const selection =
      readerSelection ?? readerRef.current?.createSelectionAnnotation();
    const text = (selection?.selectedText ?? "").trim();
    if (!text) return;
    // Define is per-word; trim to the first whitespace-delimited
    // token. Server tolerates multi-word inputs but Wiktionary
    // doesn't index them.
    const word = text.split(/\s+/, 1)[0]?.replace(/[^\p{L}\p{N}'-]/gu, "") ?? "";
    if (!word) return;
    setDefinePopover({ word });
    readerRef.current?.clearSelection();
  };

  const translateSelection = () => {
    const selection =
      readerSelection ?? readerRef.current?.createSelectionAnnotation();
    const text = (selection?.selectedText ?? "").trim();
    if (!text) return;
    setTranslatePopover({ text });
    readerRef.current?.clearSelection();
  };

  // Custom fonts — the user's uploaded TTF / OTF / WOFF files. We
  // inject one @font-face rule per font into a style tag so the
  // reader's existing font-family picker can name them. The URL
  // already carries the mount path; the browser caches aggressively
  // once it's pulled the bytes.
  const customFonts = useQuery({
    queryKey: ["custom-fonts"],
    queryFn: listCustomFonts,
  });
  useEffect(() => {
    const items = customFonts.data?.items ?? [];
    if (items.length === 0) return;
    const tag = document.createElement("style");
    tag.setAttribute("data-silo-custom-fonts", "1");
    tag.textContent = items
      .map(
        (f) =>
          // f.url is relative to the plugin's API root (/me/fonts/...).
          // mountPath() returns /api/v1/plugins/{installId}, so we add
          // /api/v1 between them to reach the actual data endpoint via
          // the host proxy.
          `@font-face { font-family: ${JSON.stringify(f.name)}; src: url(${JSON.stringify(
            `${mountPath()}/api/v1${f.url}`,
          )}); font-display: swap; }`,
      )
      .join("\n");
    document.head.appendChild(tag);
    return () => {
      document.head.removeChild(tag);
    };
  }, [customFonts.data?.items]);

  useEffect(() => {
    const files = detail.data?.files ?? [];
    if (!files.length) return;
    // Prefer EPUB (the only format the inline reader actually supports). If
    // the book ships only PDF/CBZ/etc., default to the first available format
    // — the reader page detects that and shows a download-to-read panel.
    const preferred =
      files.find((file) => file.format.toLowerCase() === "epub") ?? files[0];
    setSelectedFormat(preferred.format.toLowerCase());
  }, [detail.data?.files]);

  useEffect(() => {
    if (!id) return;
    let cancelled = false;
    void getReaderConfig(id)
      .then((envelope) => {
        if (cancelled) return;
        const viewSettings = (envelope.config?.viewSettings ?? {}) as Record<
          string,
          unknown
        >;
        const external = envelope.config?.externalProgress;
        setExternalProgress(
          external && typeof external === "object"
            ? (external as ExternalReaderProgress)
            : null,
        );
        if (typeof viewSettings.flow === "string") {
          setFlow(viewSettings.flow as ReaderFlow);
        }
        if (typeof viewSettings.fontFamily === "string") {
          setFontFamily(viewSettings.fontFamily);
        }
        if (typeof viewSettings.fontSize === "number") {
          setFontSize(viewSettings.fontSize);
        }
        if (typeof viewSettings.fontWeight === "number") {
          setFontWeight(viewSettings.fontWeight);
        }
        if (typeof viewSettings.hyphenation === "boolean") {
          setHyphenation(viewSettings.hyphenation);
        }
        if (Array.isArray(viewSettings.customHighlightColors)) {
          setCustomHighlightColors(
            viewSettings.customHighlightColors.filter(
              (value): value is string => typeof value === "string",
            ),
          );
        }
        if (typeof viewSettings.lineHeight === "number") {
          setLineHeight(viewSettings.lineHeight);
        }
        if (typeof viewSettings.margin === "number") {
          setMargin(viewSettings.margin);
        }
        if (typeof viewSettings.maxWidth === "number") {
          setMaxWidth(viewSettings.maxWidth);
        }
        if (typeof viewSettings.fontBrightness === "number") {
          setFontBrightness(
            Math.min(200, Math.max(40, viewSettings.fontBrightness)),
          );
        }
        if (typeof viewSettings.quickAction === "string") {
          setQuickAction(viewSettings.quickAction as QuickAction);
        }
        if (typeof viewSettings.readingRuler === "boolean") {
          setReadingRuler(viewSettings.readingRuler);
        }
        if (typeof viewSettings.rulerTop === "number") {
          setRulerTop(Math.min(100, Math.max(0, viewSettings.rulerTop)));
        }
        if (typeof viewSettings.rtl === "boolean") {
          setRtl(viewSettings.rtl);
        }
        if (typeof viewSettings.spread === "string") {
          setSpread(viewSettings.spread as "auto" | "none");
        }
        if (typeof viewSettings.theme === "string") {
          setTheme(viewSettings.theme as ReaderTheme);
        }
        if (typeof viewSettings.ttsRate === "number") {
          setTtsRate(viewSettings.ttsRate);
        }
        if (typeof viewSettings.ttsSleepMinutes === "number") {
          setTtsSleepMinutes(viewSettings.ttsSleepMinutes);
        }
        if (typeof viewSettings.ttsVoice === "string") {
          setTtsVoice(viewSettings.ttsVoice);
        }
        if (typeof viewSettings.writingMode === "string") {
          setWritingMode(viewSettings.writingMode as WritingMode);
        }
        if (typeof viewSettings.zoom === "number") {
          setZoom(viewSettings.zoom);
        }
      })
      .finally(() => {
        if (!cancelled) setSettingsLoaded(true);
      });
    return () => {
      cancelled = true;
      setSettingsLoaded(false);
    };
  }, [id]);

  // Show a one-time "Progress saved" toast on the first successful save after
  // the reader mounts. Subsequent saves are silent — toasts every 500ms would
  // be noise. The flag resets when the book id changes.
  const firstSaveToastedRef = useRef(false);
  useEffect(() => {
    firstSaveToastedRef.current = false;
  }, [id]);
  useEffect(() => {
    if (!id || !settingsLoaded) return;
    const timeout = window.setTimeout(() => {
      void getReaderConfig(id)
        .then((envelope) => {
          const persistedConfig = { ...(envelope.config ?? {}) };
          delete persistedConfig.externalProgress;
          return putReaderConfig(id, {
            ...persistedConfig,
            viewSettings: {
              ...((envelope.config?.viewSettings ?? {}) as Record<
                string,
                unknown
              >),
              flow,
              fontBrightness,
              fontFamily,
              fontSize,
              fontWeight,
              customHighlightColors,
              hyphenation,
              lineHeight,
              margin,
              maxWidth,
              quickAction,
              readingRuler,
              rulerTop,
              rtl,
              spread,
              theme,
              ttsRate,
              ttsSleepMinutes,
              ttsVoice,
              writingMode,
              zoom,
            },
          });
        })
        .then(() => {
          if (!firstSaveToastedRef.current) {
            firstSaveToastedRef.current = true;
            toast.success('Progress saved — autosaving as you read');
          }
        })
        .catch(() => {
          // Save failed: tell the user once so they know their position
          // might not persist (we keep retrying every change).
          if (!firstSaveToastedRef.current) {
            firstSaveToastedRef.current = true;
            toast.error('Could not save reading progress');
          }
        });
    }, 500);
    return () => window.clearTimeout(timeout);
  }, [
    flow,
    fontBrightness,
    fontFamily,
    fontSize,
    fontWeight,
    customHighlightColors,
    hyphenation,
    id,
    lineHeight,
    margin,
    maxWidth,
    quickAction,
    readingRuler,
    rulerTop,
    rtl,
    settingsLoaded,
    spread,
    theme,
    ttsRate,
    ttsSleepMinutes,
    ttsVoice,
    writingMode,
    zoom,
  ]);

  const handleProgress = useCallback(
    (progress: { cfi: string; percentage: number }) => {
      const nextPct = Math.round(progress.percentage * 100);
      setPct(nextPct);
      setScrubPct(nextPct);
      setCurrentCfi(progress.cfi);
    },
    [],
  );

  const handleReady = useCallback(
    (data: { readaloud: boolean; toc: TOCItem[] }) => {
      setReadaloudAvailable(data.readaloud);
      setToc(data.toc);
    },
    [],
  );

  const handleDiagnostic = useCallback((entry: ReaderDiagnostic) => {
    setDiagnostics((items) => [entry, ...items].slice(0, 25));
    // Until now error-level diagnostics were silently appended to a buried
    // panel section, so a user whose font failed to load or whose annotation
    // restore broke saw no feedback. Surface those via toast; warnings stay
    // quiet because they're noisier and the panel still shows the full log.
    if (entry.level === "error") {
      toast.error(`Reader: ${entry.message}`);
    }
  }, []);

  // Prefer the portal-signed file URL from detail.files[].url — it embeds a
  // short-TTL token that lets the reader fetch bytes without sending an
  // Authorization header. Fall back to the portal proxy endpoint for
  // back-compat with backends that don't yet emit stream_url.
  const fileURL =
    detail.data?.files?.find(
      (file) => file.format.toLowerCase() === selectedFormat,
    )?.url ??
    `${mountPath()}/api/v1/me/books/${encodeURIComponent(id)}/file?format=${encodeURIComponent(selectedFormat)}`;
  const normalizedFormat = selectedFormat.toLowerCase();
  // Only EPUB renders inline via foliate-js. PDF, comic-book archives, and
  // anything else fall through to the "download to read elsewhere" panel
  // (previously the reader would mount, fail silently, and show a useless
  // "Open file" link). Once PDF.js / comic rendering is built, expand this
  // set and the reader UI can pick the right viewer.
  const inlineSupportedFormats = new Set(["epub"]);
  const canUseReader =
    !!selectedFormat && inlineSupportedFormats.has(normalizedFormat);

  const saveNote = async () => {
    if (!noteText.trim()) return;
    if (editingAnnotation) {
      const selection =
        readerSelection ?? readerRef.current?.createSelectionAnnotation();
      await updateAnnotation(editingAnnotation.id, {
        cfi_range: selection?.cfi,
        color: highlightColor,
        note_text: noteText.trim(),
        selected_text: selection?.selectedText,
        style: highlightStyle,
      });
      setEditingAnnotation(null);
      setNoteText("");
      setPendingNoteSelection(null);
      readerRef.current?.clearSelection();
      await annotations.refetch();
      return;
    }
    const selection =
      pendingNoteSelection ?? readerRef.current?.createSelectionAnnotation();
    const cfi = selection?.cfi || currentCfi;
    if (!cfi) return;
    await createAnnotation(id, {
      cfi_range: cfi,
      kind: "note",
      color: "#facc15",
      readest_type: "annotation",
      selected_text: selection?.selectedText,
      note_text: noteText.trim(),
    });
    setPendingNoteSelection(null);
    setNoteText("");
    readerRef.current?.clearSelection();
    await annotations.refetch();
  };

  const saveHighlight = async (
    style = highlightStyle,
    color = highlightColor,
    selection = readerSelection ??
      readerRef.current?.createSelectionAnnotation(),
  ) => {
    if (!selection) return;
    await createAnnotation(id, {
      cfi_range: selection.cfi,
      kind: "highlight",
      color,
      readest_type: "annotation",
      selected_text: selection.selectedText,
      style,
    });
    setCurrentCfi(selection.cfi);
    readerRef.current?.clearSelection();
    await annotations.refetch();
    setPanelTab("notes");
    setShowPanel(true);
  };

  const runQuickAction = () => {
    if (quickAction === "highlight") {
      void saveHighlight();
    } else if (quickAction === "note") {
      startNoteFromSelection();
    } else if (quickAction === "search") {
      void searchSelection();
    } else {
      speakSelection();
    }
  };

  const startNoteFromSelection = () => {
    const selection =
      readerSelection ?? readerRef.current?.createSelectionAnnotation();
    if (!selection) return;
    setPendingNoteSelection(selection);
    setCurrentCfi(selection.cfi);
    setEditingAnnotation(null);
    setNoteText("");
    setPanelTab("notes");
    setShowPanel(true);
  };

  const editAnnotation = (annotation: Annotation) => {
    setEditingAnnotation(annotation);
    setNoteText(annotation.note_text || "");
    setHighlightColor(annotation.color || "#facc15");
    setHighlightStyle((annotation.style as HighlightStyle) || "highlight");
    setPanelTab("notes");
    setShowPanel(true);
    navigateTo(annotation.cfi_range);
  };

  const removeAnnotation = async (annotation: Annotation) => {
    await deleteAnnotation(annotation.id);
    if (editingAnnotation?.id === annotation.id) {
      setEditingAnnotation(null);
      setNoteText("");
      setPendingNoteSelection(null);
    }
    await annotations.refetch();
  };

  const replaceAnnotationRange = async (annotation: Annotation) => {
    const selection =
      readerSelection ?? readerRef.current?.createSelectionAnnotation();
    if (!selection) return;
    await updateAnnotation(annotation.id, {
      cfi_range: selection.cfi,
      color: annotation.color,
      note_text: annotation.note_text,
      selected_text: selection.selectedText,
      style: annotation.style,
    });
    readerRef.current?.clearSelection();
    await annotations.refetch();
  };

  const saveBookmark = async () => {
    if (!currentCfi) return;
    await createAnnotation(id, {
      cfi_range: currentCfi,
      kind: "bookmark",
      readest_type: "bookmark",
      note_text: "",
    });
    await annotations.refetch();
    setPanelTab("bookmarks");
    setShowPanel(true);
  };

  const runSearchFor = async (query: string) => {
    const term = query.trim();
    if (!term) {
      readerRef.current?.clearSearch();
      setSearchResults([]);
      setSearchResultIndex(0);
      return;
    }
    setSearching(true);
    try {
      const results = await readerRef.current?.search(term, searchOptions);
      const nextResults = results ?? [];
      setSearchResults(nextResults);
      setSearchResultIndex(0);
      if (nextResults[0]) {
        navigateTo(nextResults[0].cfi);
      }
    } finally {
      setSearching(false);
    }
  };

  const runSearch = async () => {
    await runSearchFor(searchTerm);
  };

  const searchSelection = async () => {
    const selection =
      readerSelection ?? readerRef.current?.createSelectionAnnotation();
    if (!selection) return;
    setSearchTerm(selection.selectedText);
    setPanelTab("search");
    setShowPanel(true);
    await runSearchFor(selection.selectedText);
  };

  const navigateTo = (target: string) => {
    if (!target) return;
    readerRef.current?.goTo(target);
  };

  const scrubTo = (value: number) => {
    const clamped = Math.min(Math.max(value, 0), 100);
    setScrubPct(clamped);
    void readerRef.current?.goToFraction(clamped / 100);
  };

  const navigateSearchResult = (direction: -1 | 1) => {
    if (!searchResults.length) return;
    const nextIndex =
      (searchResultIndex + direction + searchResults.length) %
      searchResults.length;
    setSearchResultIndex(nextIndex);
    navigateTo(searchResults[nextIndex].cfi);
  };

  const splitReadableChunks = (text: string) =>
    text
      .replace(/\s+/g, " ")
      .split(/(?<=[.!?])\s+/)
      .map((chunk) => chunk.trim())
      .filter(Boolean);

  const speakText = (text?: string, forcedIndex?: number) => {
    if (!("speechSynthesis" in window)) return;
    const content = text ?? readerRef.current?.getReadableText();
    if (!content) return;
    const chunks = splitReadableChunks(content);
    const index = forcedIndex ?? (text ? 0 : ttsChunkIndex);
    const current = chunks[index] ?? chunks[0] ?? content;
    const utterance = new SpeechSynthesisUtterance(current);
    utterance.rate = ttsRate;
    const voice = ttsVoices.find((item) => item.voiceURI === ttsVoice);
    if (voice) utterance.voice = voice;
    utterance.onend = () => {
      setTtsActive(false);
      setTtsPaused(false);
      setTtsChunks(chunks);
      setTtsChunkIndex(index);
    };
    utterance.onerror = () => setTtsActive(false);
    window.speechSynthesis.cancel();
    window.speechSynthesis.speak(utterance);
    setTtsChunks(chunks);
    setTtsChunkIndex(index);
    setTtsPaused(false);
    setTtsActive(true);
    if (ttsSleepMinutes > 0) {
      window.setTimeout(
        () => window.speechSynthesis.cancel(),
        ttsSleepMinutes * 60 * 1000,
      );
    }
  };

  const toggleTts = () => {
    if (ttsActive) {
      window.speechSynthesis.cancel();
      setTtsActive(false);
      setTtsPaused(false);
      return;
    }
    speakText();
  };

  const pauseResumeTts = () => {
    if (!("speechSynthesis" in window)) return;
    if (ttsPaused) {
      window.speechSynthesis.resume();
      setTtsPaused(false);
    } else {
      window.speechSynthesis.pause();
      setTtsPaused(true);
    }
  };

  const jumpTtsChunk = (direction: -1 | 1) => {
    const chunks = ttsChunks.length
      ? ttsChunks
      : splitReadableChunks(readerRef.current?.getReadableText() ?? "");
    if (!chunks.length) return;
    const nextIndex = Math.min(
      Math.max(ttsChunkIndex + direction, 0),
      chunks.length - 1,
    );
    setTtsChunks(chunks);
    setTtsChunkIndex(nextIndex);
    speakText(chunks.join(" "), nextIndex);
  };

  const speakSelection = () => {
    const selection =
      readerSelection ?? readerRef.current?.createSelectionAnnotation();
    if (!selection) return;
    speakText(selection.selectedText);
  };

  const copySelection = async () => {
    const selection =
      readerSelection ?? readerRef.current?.createSelectionAnnotation();
    if (!selection) return;
    await navigator.clipboard?.writeText(selection.selectedText);
    readerRef.current?.clearSelection();
  };

  const updateSearchOption = <K extends keyof Required<ReaderSearchOptions>>(
    key: K,
    value: Required<ReaderSearchOptions>[K],
  ) => {
    setSearchOptions((current) => ({ ...current, [key]: value }));
  };

  const startRsvp = () => {
    const words = (readerRef.current?.getReadableText() ?? "")
      .replace(/\s+/g, " ")
      .split(" ")
      .filter(Boolean);
    if (!words.length) return;
    setRsvpWords(words);
    setRsvpIndex(0);
    setRsvpActive(true);
  };

  const exportNotes = (plainText: boolean) => {
    const lines = [
      detail.data?.title || "Reader notes",
      (detail.data?.authors ?? []).join(", "),
      "",
      ...noteItems.map((annotation) => {
        const quote = annotation.selected_text || annotation.cfi_range;
        const note = annotation.note_text
          ? `\nNote: ${annotation.note_text}`
          : "";
        return plainText
          ? `${quote}${note}`
          : `> ${quote}\n${note ? `\n${note}` : ""}`;
      }),
    ];
    const blob = new Blob([lines.join("\n\n")], {
      type: plainText ? "text/plain" : "text/markdown",
    });
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = `${detail.data?.title || "reader-notes"}.${
      plainText ? "txt" : "md"
    }`;
    link.click();
    URL.revokeObjectURL(url);
  };

  const saveKosyncLink = async () => {
    const document = kosyncDocument.trim();
    if (!document) return;
    await linkKosyncBook(id, { document, format: selectedFormat });
    const envelope = await getReaderConfig(id);
    const external = envelope.config?.externalProgress;
    setExternalProgress(
      external && typeof external === "object"
        ? (external as ExternalReaderProgress)
        : null,
    );
    setKosyncDocument("");
  };

  const currentReaderDefaults = () => ({
    flow,
    fontBrightness,
    fontFamily,
    fontSize,
    fontWeight,
    customHighlightColors,
    hyphenation,
    lineHeight,
    margin,
    maxWidth,
    quickAction,
    readingRuler,
    rulerTop,
    rtl,
    spread,
    theme,
    ttsRate,
    ttsSleepMinutes,
    ttsVoice,
    writingMode,
    zoom,
  });

  const applyReaderDefaults = (defaults: Record<string, unknown>) => {
    if (typeof defaults.flow === "string") setFlow(defaults.flow as ReaderFlow);
    if (typeof defaults.fontFamily === "string")
      setFontFamily(defaults.fontFamily);
    if (typeof defaults.fontSize === "number") setFontSize(defaults.fontSize);
    if (typeof defaults.fontWeight === "number")
      setFontWeight(defaults.fontWeight);
    if (Array.isArray(defaults.customHighlightColors)) {
      setCustomHighlightColors(
        defaults.customHighlightColors.filter(
          (value): value is string => typeof value === "string",
        ),
      );
    }
    if (typeof defaults.hyphenation === "boolean")
      setHyphenation(defaults.hyphenation);
    if (typeof defaults.lineHeight === "number")
      setLineHeight(defaults.lineHeight);
    if (typeof defaults.margin === "number") setMargin(defaults.margin);
    if (typeof defaults.maxWidth === "number") setMaxWidth(defaults.maxWidth);
    if (typeof defaults.fontBrightness === "number")
      setFontBrightness(Math.min(200, Math.max(40, defaults.fontBrightness)));
    if (typeof defaults.quickAction === "string")
      setQuickAction(defaults.quickAction as QuickAction);
    if (typeof defaults.readingRuler === "boolean")
      setReadingRuler(defaults.readingRuler);
    if (typeof defaults.rulerTop === "number")
      setRulerTop(Math.min(100, Math.max(0, defaults.rulerTop)));
    if (typeof defaults.rtl === "boolean") setRtl(defaults.rtl);
    if (typeof defaults.spread === "string")
      setSpread(defaults.spread as "auto" | "none");
    if (typeof defaults.theme === "string")
      setTheme(defaults.theme as ReaderTheme);
    if (typeof defaults.ttsRate === "number") setTtsRate(defaults.ttsRate);
    if (typeof defaults.ttsSleepMinutes === "number")
      setTtsSleepMinutes(defaults.ttsSleepMinutes);
    if (typeof defaults.ttsVoice === "string") setTtsVoice(defaults.ttsVoice);
    if (typeof defaults.writingMode === "string")
      setWritingMode(defaults.writingMode as WritingMode);
    if (typeof defaults.zoom === "number") setZoom(defaults.zoom);
  };

  const saveGlobalDefaults = () => {
    localStorage.setItem(
      globalReaderDefaultsKey,
      JSON.stringify(currentReaderDefaults()),
    );
  };

  const loadGlobalDefaults = () => {
    const raw = localStorage.getItem(globalReaderDefaultsKey);
    if (!raw) return;
    applyReaderDefaults(JSON.parse(raw) as Record<string, unknown>);
  };

  const exportAnnotationsJson = () => {
    const blob = new Blob(
      [
        JSON.stringify(
          {
            book_id: id,
            exported_at: new Date().toISOString(),
            annotations: annotations.data?.items ?? [],
          },
          null,
          2,
        ),
      ],
      { type: "application/json" },
    );
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = `${detail.data?.title || "reader-annotations"}.json`;
    link.click();
    URL.revokeObjectURL(url);
  };

  const importAnnotationsJson = async (file: File | undefined) => {
    if (!file) return;
    const payload = JSON.parse(await file.text()) as {
      annotations?: Partial<Annotation>[];
    };
    for (const annotation of payload.annotations ?? []) {
      if (!annotation.cfi_range || !annotation.kind) continue;
      await createAnnotation(id, {
        cfi_range: annotation.cfi_range,
        kind: annotation.kind,
        color: annotation.color,
        selected_text: annotation.selected_text,
        note_text: annotation.note_text,
        readest_type: annotation.readest_type,
        style: annotation.style,
        page: annotation.page,
        metadata_json: annotation.metadata_json,
      });
    }
    await annotations.refetch();
  };

  useEffect(() => {
    return () => window.speechSynthesis?.cancel();
  }, []);

  useEffect(() => {
    if (!("speechSynthesis" in window)) return;
    const loadVoices = () => setTtsVoices(window.speechSynthesis.getVoices());
    loadVoices();
    window.speechSynthesis.addEventListener("voiceschanged", loadVoices);
    return () =>
      window.speechSynthesis.removeEventListener("voiceschanged", loadVoices);
  }, []);

  useEffect(() => {
    if (!rsvpActive || !rsvpWords.length) return;
    const timeout = window.setTimeout(
      () =>
        setRsvpIndex((value) => {
          if (value >= rsvpWords.length - 1) {
            setRsvpActive(false);
            return value;
          }
          return value + 1;
        }),
      Math.max(80, 60000 / rsvpWpm),
    );
    return () => window.clearTimeout(timeout);
  }, [rsvpActive, rsvpIndex, rsvpWords.length, rsvpWpm]);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement | null;
      if (
        target?.closest(
          "input, textarea, select, button, [contenteditable='true']",
        )
      ) {
        return;
      }
      if (event.key === "ArrowRight" || event.key === " ") {
        event.preventDefault();
        readerRef.current?.next();
      } else if (event.key === "ArrowLeft") {
        event.preventDefault();
        readerRef.current?.prev();
      } else if (event.key.toLowerCase() === "b") {
        event.preventDefault();
        void saveBookmark();
      } else if (event.key.toLowerCase() === "h") {
        event.preventDefault();
        void saveHighlight();
      } else if (event.key.toLowerCase() === "n") {
        event.preventDefault();
        startNoteFromSelection();
      } else if (event.key.toLowerCase() === "f") {
        event.preventDefault();
        setPanelTab("search");
        setShowPanel(true);
      } else if (event.key.toLowerCase() === "p") {
        event.preventDefault();
        setShowPanel((value) => !value);
      } else if (event.key.toLowerCase() === "t") {
        event.preventDefault();
        toggleTts();
      } else if (event.key === "Escape") {
        readerRef.current?.clearSelection();
        setContentPopup(null);
        setRsvpActive(false);
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  });

  const toggleFullscreen = () => {
    if (document.fullscreenElement) {
      void document.exitFullscreen();
      return;
    }
    void document.documentElement.requestFullscreen?.();
  };

  const renderTocItems = (items: TOCItem[], depth = 0) =>
    items.map((item) => (
      <div key={`${item.href}-${item.id}`}>
        <button
          type="button"
          onClick={() => navigateTo(item.cfi || item.href)}
          className="block w-full rounded-md px-2 py-1.5 text-left text-sm hover:bg-accent"
          style={{ paddingLeft: `${8 + depth * 14}px` }}
        >
          <span className="line-clamp-2">{item.label || item.href}</span>
        </button>
        {item.subitems?.length
          ? renderTocItems(item.subitems, depth + 1)
          : null}
      </div>
    ));

  const bookmarkItems = (annotations.data?.items ?? []).filter(
    (annotation) =>
      annotation.kind === "bookmark" || annotation.readest_type === "bookmark",
  );
  const noteItems = (annotations.data?.items ?? []).filter(
    (annotation) =>
      annotation.kind !== "bookmark" && annotation.readest_type !== "bookmark",
  );
  const filteredNoteItems = noteSearchTerm.trim()
    ? noteItems.filter((annotation) =>
        [
          annotation.note_text,
          annotation.selected_text,
          annotation.cfi_range,
          annotation.kind,
        ]
          .join(" ")
          .toLowerCase()
          .includes(noteSearchTerm.trim().toLowerCase()),
      )
    : noteItems;
  // Group notes/highlights by the TOC entry they fall into so the panel reads
  // like an outline rather than a flat dump. foliate-js writes a `cfi` on each
  // TOC item at load time; we flatten the tree, sort by CFI, then for each
  // annotation pick the latest TOC entry whose CFI <= the annotation's start.
  const flatToc: { label: string; cfi: string }[] = [];
  const walkToc = (items: TOCItem[]) => {
    for (const item of items) {
      if (item.cfi) flatToc.push({ label: item.label, cfi: item.cfi });
      if (item.subitems?.length) walkToc(item.subitems);
    }
  };
  walkToc(toc);
  try {
    flatToc.sort((a, b) => CFI.compare(a.cfi, b.cfi));
  } catch {
    // CFI.compare throws on malformed input; skip sort and fall through to the
    // "Unknown chapter" bucket rather than break the panel.
  }
  const chapterFor = (cfi: string): string => {
    if (!flatToc.length || !cfi) return "Unknown chapter";
    let label = flatToc[0].label;
    try {
      for (const entry of flatToc) {
        if (CFI.compare(entry.cfi, cfi) <= 0) label = entry.label;
        else break;
      }
    } catch {
      return "Unknown chapter";
    }
    return label;
  };
  const noteGroups: { label: string; items: typeof filteredNoteItems }[] = [];
  for (const annotation of filteredNoteItems) {
    const label = chapterFor(annotation.cfi_range);
    const bucket = noteGroups.find((g) => g.label === label);
    if (bucket) bucket.items.push(annotation);
    else noteGroups.push({ label, items: [annotation] });
  }
  const allHighlightColors = [
    ...highlightColors,
    ...customHighlightColors.map((value) => ({ label: value, value })),
  ];

  // On phones the floating toolbar lands under the on-screen keyboard or off
  // the right edge in landscape. Detect the narrow viewport and skip the
  // computed style — the className pins the bar to the bottom of the viewport
  // and lets controls wrap. Above sm: keep the original "floats near the
  // selection" behavior since there's room.
  const isNarrowViewport =
    typeof window !== "undefined" && window.innerWidth < 640;
  const toolbarStyle =
    readerSelection && !isNarrowViewport
      ? {
          left: `${Math.min(
            Math.max(
              readerSelection.rect.left + readerSelection.rect.width / 2,
              180,
            ),
            window.innerWidth - 180,
          )}px`,
          top: `${Math.max(readerSelection.rect.top - 58, 72)}px`,
          transform: "translateX(-50%)",
        }
      : undefined;

  return (
    <div className="-mx-4 -my-2 flex h-[calc(100dvh-3.5rem)] flex-col md:-mx-6 lg:-mx-8">
      <header className="flex flex-wrap items-center gap-2 border-b border-border bg-card px-4 py-2">
        <Button asChild size="sm" variant="ghost">
          <Link to={`/${encodeURIComponent(id)}`}>
            <ChevronLeft className="mr-1 size-4" /> Back
          </Link>
        </Button>
        <div className="min-w-0 flex-1">
          <div className="truncate text-sm font-medium">
            {detail.data?.title || "Reader"}
          </div>
          <div className="truncate text-xs text-muted-foreground">
            {[...(detail.data?.authors ?? []), currentSection]
              .filter(Boolean)
              .join(" - ")}
          </div>
        </div>
        {detail.data?.files?.length ? (
          <select
            value={selectedFormat}
            onChange={(e) => setSelectedFormat(e.target.value)}
            className="h-9 rounded-md border border-border bg-background px-2 text-sm"
            aria-label="Reader format"
          >
            {detail.data.files.map((file) => (
              <option key={file.format} value={file.format.toLowerCase()}>
                {file.format.toUpperCase()}
              </option>
            ))}
          </select>
        ) : null}
        {canUseReader && (
          <>
            <div className="flex items-center gap-1 rounded-md border border-border bg-background px-1 py-1">
              <Type className="ml-1 size-4 text-muted-foreground" />
              <Button
                size="sm"
                variant="ghost"
                onClick={() => setFontSize((value) => Math.max(75, value - 10))}
              >
                A-
              </Button>
              <span className="w-10 text-center text-xs text-muted-foreground">
                {fontSize}%
              </span>
              <Button
                size="sm"
                variant="ghost"
                onClick={() =>
                  setFontSize((value) => Math.min(160, value + 10))
                }
              >
                A+
              </Button>
            </div>
            <select
              value={theme}
              onChange={(e) =>
                setTheme(e.target.value as "light" | "sepia" | "dark")
              }
              className="h-9 rounded-md border border-border bg-background px-2 text-sm"
              aria-label="Reader theme"
            >
              <option value="light">Light</option>
              <option value="sepia">Sepia</option>
              <option value="dark">Dark</option>
            </select>
            <Button
              size="icon"
              variant={spread === "none" ? "secondary" : "ghost"}
              onClick={() =>
                setSpread((value) => (value === "none" ? "auto" : "none"))
              }
              aria-label="Toggle page spread"
            >
              <Columns2 className="size-4" />
            </Button>
            <Button
              size="icon"
              variant={readingRuler ? "secondary" : "ghost"}
              onClick={() => setReadingRuler((value) => !value)}
              aria-label="Toggle reading ruler"
              title="Reading ruler — dims everything except the current line, useful for focus"
            >
              <Ruler className="size-4" />
            </Button>
            <div
              className="flex items-center gap-1 rounded-md border border-border bg-background px-1 py-1"
              title="Font brightness — dims text toward the background for less eye strain"
            >
              <Sun className="ml-1 size-4 text-muted-foreground" />
              <Button
                size="sm"
                variant="ghost"
                onClick={() =>
                  setFontBrightness((value) => Math.max(40, value - 10))
                }
                aria-label="Decrease font brightness"
              >
                B-
              </Button>
              <span className="w-10 text-center text-xs text-muted-foreground">
                {fontBrightness}%
              </span>
              <Button
                size="sm"
                variant="ghost"
                onClick={() =>
                  setFontBrightness((value) => Math.min(200, value + 10))
                }
                aria-label="Increase font brightness"
              >
                B+
              </Button>
            </div>
            <Button
              size="sm"
              variant={showPanel ? "secondary" : "ghost"}
              onClick={() => setShowPanel((value) => !value)}
            >
              Panel
            </Button>
            <Button
              size="icon"
              variant="ghost"
              onClick={() => void saveBookmark()}
              disabled={!currentCfi}
              aria-label="Add bookmark"
            >
              <Bookmark className="size-4" />
            </Button>
            <Button
              size="icon"
              variant="ghost"
              onClick={() => void saveHighlight()}
              aria-label="Highlight selection"
            >
              <Highlighter className="size-4" />
            </Button>
            <Button
              size="icon"
              variant="ghost"
              onClick={toggleFullscreen}
              aria-label="Toggle fullscreen"
            >
              <Maximize2 className="size-4" />
            </Button>
          </>
        )}
        <div className="flex-1" />
        <label className="hidden min-w-40 items-center gap-2 text-xs text-muted-foreground md:flex">
          <span className="w-8 text-right">{pct}%</span>
          <input
            type="range"
            min="0"
            max="100"
            step="1"
            value={scrubPct}
            onChange={(event) => scrubTo(Number(event.target.value))}
            aria-label="Reading progress"
            className="w-28"
          />
        </label>
        <span className="text-xs text-muted-foreground md:hidden">{pct}%</span>
        {canUseReader && (
          <>
            <Button
              size="icon"
              variant="ghost"
              onClick={() => readerRef.current?.prev()}
              aria-label="Previous"
            >
              <ArrowLeft className="size-4" />
            </Button>
            <Button
              size="icon"
              variant="ghost"
              onClick={() => readerRef.current?.next()}
              aria-label="Next"
            >
              <ArrowRight className="size-4" />
            </Button>
          </>
        )}
      </header>
      {(externalProgress || readaloudAvailable) && (
        <div className="flex flex-wrap items-center gap-2 border-b border-border bg-muted/40 px-4 py-2 text-xs text-muted-foreground">
          <span className="font-medium text-foreground">Book mode</span>
          {readaloudAvailable ? (
            <span>
              Readaloud metadata detected; web speech remains available.
            </span>
          ) : null}
          {externalProgress ? (
            <div className="flex flex-wrap items-center gap-2">
              <span>
                KOReader {Math.round((externalProgress.percentage ?? 0) * 100)}%
                from {externalProgress.device || "external reader"}
              </span>
              {externalProgress.canResume && externalProgress.location ? (
                <Button
                  size="sm"
                  variant="secondary"
                  onClick={() => navigateTo(externalProgress.location ?? "")}
                >
                  Resume external
                </Button>
              ) : (
                <span>External progress is linked but not CFI-resumable.</span>
              )}
            </div>
          ) : null}
        </div>
      )}
      <div className="grid min-h-0 flex-1 grid-cols-1 lg:grid-cols-[1fr_20rem]">
        {canUseReader ? (
          <ReadestLiteReader
            ref={readerRef}
            annotations={annotations.data?.items ?? []}
            key={`${id}-${selectedFormat}`}
            bookID={id}
            format={selectedFormat}
            fileUrl={fileURL}
            settings={{
              flow,
              fontBrightness,
              fontFamily,
              fontSize,
              fontWeight,
              hyphenation,
              lineHeight,
              margin,
              maxWidth,
              rtl,
              spread,
              theme,
              writingMode,
              zoom,
            }}
            onContentPopup={setContentPopup}
            onDiagnostic={handleDiagnostic}
            onReady={handleReady}
            onProgress={handleProgress}
            onSectionChange={setCurrentSection}
            onSelectionChange={setReaderSelection}
          />
        ) : (
          <div className="flex h-full flex-col items-center justify-center gap-4 p-6 text-center">
            <div className="text-lg font-semibold">
              Read this {selectedFormat.toUpperCase()} in your preferred app
            </div>
            <p className="max-w-md text-sm text-muted-foreground">
              The in-browser reader currently supports EPUB. Download the file
              and open it in a dedicated app (Adobe Acrobat for PDFs, Panels /
              Chunky for comics, etc.) to read with proper navigation and
              annotation support.
            </p>
            <Button asChild>
              <a href={fileURL} download>
                Download {selectedFormat.toUpperCase()}
              </a>
            </Button>
          </div>
        )}
        {showPanel && canUseReader && (
          <aside className="min-h-0 overflow-y-auto border-l border-border bg-card p-4">
            <div className="grid grid-cols-5 gap-1 rounded-md border border-border bg-background p-1">
              {(
                [
                  { id: "toc", label: "TOC", icon: ListTree },
                  { id: "search", label: "Find", icon: Search },
                  { id: "notes", label: "Notes", icon: NotebookPen },
                  { id: "bookmarks", label: "Marks", icon: Bookmark },
                  { id: "settings", label: "Settings", icon: Settings },
                ] satisfies Array<{
                  id: ReaderPanelTab;
                  label: string;
                  icon: typeof ListTree;
                }>
              ).map(({ id: tabId, label, icon: Icon }) => (
                <button
                  key={tabId}
                  type="button"
                  onClick={() => setPanelTab(tabId)}
                  aria-label={label}
                  title={label}
                  className={`flex h-auto flex-col items-center justify-center gap-1 rounded-md px-1 py-1.5 text-[10px] font-medium transition-colors ${
                    panelTab === tabId
                      ? "bg-secondary text-secondary-foreground"
                      : "text-muted-foreground hover:bg-accent hover:text-accent-foreground"
                  }`}
                >
                  <Icon className="size-4" />
                  <span className="leading-none">{label}</span>
                </button>
              ))}
            </div>
            {panelTab === "toc" ? (
              <div className="mt-4 space-y-1">
                {toc.length ? (
                  renderTocItems(toc)
                ) : (
                  <p className="text-sm text-muted-foreground">
                    No table of contents found.
                  </p>
                )}
              </div>
            ) : null}
            {panelTab === "search" ? (
              <>
                <form
                  className="mt-4 flex gap-2"
                  onSubmit={(event) => {
                    event.preventDefault();
                    void runSearch();
                  }}
                >
                  <input
                    value={searchTerm}
                    onChange={(event) => setSearchTerm(event.target.value)}
                    placeholder="Search this book"
                    className="min-w-0 flex-1 rounded-md border border-border bg-background px-3 py-2 text-sm"
                  />
                  <Button size="sm" disabled={searching}>
                    {searching ? "..." : "Go"}
                  </Button>
                </form>
                <div className="mt-3 space-y-3 rounded-md border border-border bg-background p-3 text-xs">
                  <div className="grid grid-cols-2 gap-2">
                    <label className="flex items-center gap-2">
                      <input
                        type="radio"
                        name="search-scope"
                        checked={searchOptions.scope === "book"}
                        onChange={() => updateSearchOption("scope", "book")}
                      />
                      Book
                    </label>
                    <label className="flex items-center gap-2">
                      <input
                        type="radio"
                        name="search-scope"
                        checked={searchOptions.scope === "section"}
                        onChange={() => updateSearchOption("scope", "section")}
                      />
                      Section
                    </label>
                  </div>
                  <div className="grid grid-cols-1 gap-2">
                    <label className="flex items-center gap-2">
                      <input
                        type="checkbox"
                        checked={searchOptions.matchCase}
                        onChange={(event) =>
                          updateSearchOption("matchCase", event.target.checked)
                        }
                      />
                      Match case
                    </label>
                    <label className="flex items-center gap-2">
                      <input
                        type="checkbox"
                        checked={searchOptions.matchWholeWords}
                        onChange={(event) =>
                          updateSearchOption(
                            "matchWholeWords",
                            event.target.checked,
                          )
                        }
                      />
                      Whole words
                    </label>
                    <label className="flex items-center gap-2">
                      <input
                        type="checkbox"
                        checked={searchOptions.matchDiacritics}
                        onChange={(event) =>
                          updateSearchOption(
                            "matchDiacritics",
                            event.target.checked,
                          )
                        }
                      />
                      Match diacritics
                    </label>
                  </div>
                </div>
                {searchResults.length ? (
                  <div className="mt-3 flex items-center justify-between rounded-md border border-border bg-background px-2 py-1 text-xs">
                    <span>
                      {searchResultIndex + 1} of {searchResults.length}
                    </span>
                    <div className="flex gap-1">
                      <Button
                        size="icon"
                        variant="ghost"
                        onClick={() => navigateSearchResult(-1)}
                        aria-label="Previous search result"
                      >
                        <ChevronUp className="size-4" />
                      </Button>
                      <Button
                        size="icon"
                        variant="ghost"
                        onClick={() => navigateSearchResult(1)}
                        aria-label="Next search result"
                      >
                        <ChevronDown className="size-4" />
                      </Button>
                    </div>
                  </div>
                ) : null}
                <div className="mt-4 space-y-3">
                  {searchResults.map((result, index) => (
                    <button
                      key={`${result.cfi}-${index}`}
                      type="button"
                      onClick={() => {
                        setSearchResultIndex(index);
                        navigateTo(result.cfi);
                      }}
                      className={`block w-full rounded-md border p-3 text-left text-sm hover:bg-accent ${
                        index === searchResultIndex
                          ? "border-primary bg-accent"
                          : "border-border bg-background"
                      }`}
                    >
                      {result.label ? (
                        <div className="mb-1 text-xs font-medium text-muted-foreground">
                          {result.label}
                        </div>
                      ) : null}
                      {/*
                        Foliate-js sometimes returns 500+ char pre/post chunks
                        that overflow the card and force the user to squint.
                        Cap each side at ~80 chars with ellipsis so the match
                        is still surrounded by enough context to be useful.
                      */}
                      <div className="text-sm leading-relaxed">
                        <span className="text-muted-foreground">
                          {clipExcerpt(result.excerpt?.pre, 80, "lead")}
                        </span>
                        <mark className="rounded-sm bg-yellow-200/80 px-1 py-0.5 font-medium text-foreground">
                          {result.excerpt?.match}
                        </mark>
                        <span className="text-muted-foreground">
                          {clipExcerpt(result.excerpt?.post ?? result.cfi ?? "", 80, "trail")}
                        </span>
                      </div>
                    </button>
                  ))}
                  {!searching && searchTerm && searchResults.length === 0 ? (
                    <p className="text-sm text-muted-foreground">
                      No matches found.
                    </p>
                  ) : null}
                </div>
              </>
            ) : null}
            {panelTab === "notes" ? (
              <>
                <div className="mt-4 space-y-2">
                  {editingAnnotation ? (
                    <div className="rounded-md border border-border bg-background p-3 text-xs text-muted-foreground">
                      Editing annotation range. Select replacement text in the
                      reader before saving to move this annotation.
                    </div>
                  ) : null}
                  <textarea
                    value={noteText}
                    onChange={(e) => setNoteText(e.target.value)}
                    placeholder={
                      editingAnnotation
                        ? "Edit note"
                        : "Add a note at the current reading position"
                    }
                    className="min-h-24 w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
                  />
                  <div className="flex gap-2">
                    <Button
                      size="sm"
                      disabled={
                        !noteText.trim() || (!currentCfi && !editingAnnotation)
                      }
                      onClick={() => void saveNote()}
                    >
                      {editingAnnotation ? "Update note" : "Save note"}
                    </Button>
                    {editingAnnotation ? (
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={() => {
                          setEditingAnnotation(null);
                          setNoteText("");
                        }}
                      >
                        Cancel
                      </Button>
                    ) : null}
                  </div>
                </div>
                <div className="mt-4 flex gap-2">
                  <input
                    value={noteSearchTerm}
                    onChange={(event) => setNoteSearchTerm(event.target.value)}
                    placeholder="Search notes"
                    className="min-w-0 flex-1 rounded-md border border-border bg-background px-3 py-2 text-sm"
                  />
                  <Button
                    size="icon"
                    variant="ghost"
                    onClick={() => exportNotes(false)}
                    aria-label="Export notes as Markdown"
                  >
                    <Download className="size-4" />
                  </Button>
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() => exportNotes(true)}
                  >
                    TXT
                  </Button>
                </div>
                <div className="mt-4 space-y-5">
                  {noteGroups.length === 0 ? (
                    <p className="text-sm text-muted-foreground">
                      {noteSearchTerm.trim()
                        ? "No matches."
                        : "No notes or highlights yet."}
                    </p>
                  ) : null}
                  {noteGroups.map((group) => (
                    <section key={group.label}>
                      <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                        {group.label}
                        <span className="ml-2 font-normal normal-case tracking-normal">
                          ({group.items.length})
                        </span>
                      </h3>
                      <div className="space-y-3">
                        {group.items.map((annotation) => (
                          <div
                            key={annotation.id}
                            className="rounded-md border border-border bg-background p-3 text-sm"
                          >
                            <button
                              type="button"
                              onClick={() => navigateTo(annotation.cfi_range)}
                              className="block w-full text-left hover:text-primary"
                            >
                              <div className="font-medium">
                                {annotation.kind === "note"
                                  ? "Note"
                                  : "Highlight"}
                              </div>
                              <div className="mt-1 text-xs text-muted-foreground">
                                {annotation.note_text ||
                                  annotation.selected_text ||
                                  annotation.cfi_range}
                              </div>
                            </button>
                            <div className="mt-3 flex gap-2">
                              <Button
                                size="sm"
                                variant="ghost"
                                onClick={() => editAnnotation(annotation)}
                              >
                                Edit
                              </Button>
                              <Button
                                size="sm"
                                variant="ghost"
                                disabled={!readerSelection}
                                onClick={() =>
                                  void replaceAnnotationRange(annotation)
                                }
                              >
                                Replace range
                              </Button>
                              <Button
                                size="sm"
                                variant="ghost"
                                onClick={() => void removeAnnotation(annotation)}
                              >
                                <Trash2 className="mr-1 size-4" />
                                Delete
                              </Button>
                            </div>
                          </div>
                        ))}
                      </div>
                    </section>
                  ))}
                </div>
              </>
            ) : null}
            {panelTab === "bookmarks" ? (
              <div className="mt-4 space-y-3">
                {bookmarkItems.length ? (
                  bookmarkItems.map((annotation) => (
                    <div
                      key={annotation.id}
                      className="rounded-md border border-border bg-background p-3 text-sm"
                    >
                      <button
                        type="button"
                        onClick={() => navigateTo(annotation.cfi_range)}
                        className="block w-full text-left hover:text-primary"
                      >
                        <div className="font-medium">Bookmark</div>
                        <div className="mt-1 line-clamp-2 text-xs text-muted-foreground">
                          {annotation.cfi_range}
                        </div>
                      </button>
                      <div className="mt-3">
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={() => void removeAnnotation(annotation)}
                        >
                          <Trash2 className="mr-1 size-4" />
                          Delete
                        </Button>
                      </div>
                    </div>
                  ))
                ) : (
                  <p className="text-sm text-muted-foreground">
                    No bookmarks saved.
                  </p>
                )}
              </div>
            ) : null}
            {panelTab === "settings" ? (
              <div className="mt-4 space-y-6 text-sm">
                <section className="space-y-3">
                  <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                    Appearance
                  </h3>
                  <div className="space-y-1.5">
                    <span className="text-xs text-muted-foreground">Theme</span>
                    <div className="grid grid-cols-3 gap-2">
                      {(["light", "sepia", "dark"] as const).map((mode) => (
                        <Button
                          key={mode}
                          size="sm"
                          variant={theme === mode ? "secondary" : "outline"}
                          onClick={() => setTheme(mode)}
                        >
                          {mode === "light"
                            ? "Light"
                            : mode === "sepia"
                            ? "Sepia"
                            : "Dark"}
                        </Button>
                      ))}
                    </div>
                  </div>
                  <div className="space-y-1.5">
                    <span className="text-xs text-muted-foreground">
                      Font size {fontSize}%
                    </span>
                    <div className="flex items-center gap-2">
                      <Button
                        size="sm"
                        variant="outline"
                        className="flex-1"
                        onClick={() =>
                          setFontSize((value) => Math.max(75, value - 10))
                        }
                      >
                        A-
                      </Button>
                      <Button
                        size="sm"
                        variant="outline"
                        className="flex-1"
                        onClick={() =>
                          setFontSize((value) => Math.min(160, value + 10))
                        }
                      >
                        A+
                      </Button>
                    </div>
                  </div>
                  <label className="space-y-1.5">
                    <span className="text-xs text-muted-foreground">
                      Font brightness {fontBrightness}%
                    </span>
                    <input
                      type="range"
                      min="40"
                      max="200"
                      step="5"
                      value={fontBrightness}
                      onChange={(event) =>
                        setFontBrightness(Number(event.target.value))
                      }
                      className="w-full"
                    />
                  </label>
                </section>

                <section className="space-y-3">
                  <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                    Reading profile
                  </h3>
                  <div className="grid gap-2">
                    {readerPresets.map((preset) => (
                      <button
                        key={preset.id}
                        type="button"
                        onClick={() => {
                          setFontFamily(preset.fontFamily);
                          setFontSize(preset.fontSize);
                          setLineHeight(preset.lineHeight);
                        }}
                        className="rounded-md border border-border bg-background px-3 py-2 text-left hover:border-primary hover:bg-accent"
                      >
                        <div className="text-sm font-medium">{preset.label}</div>
                        <div className="text-xs text-muted-foreground">
                          {preset.description}
                        </div>
                      </button>
                    ))}
                  </div>
                </section>

                <section className="space-y-3">
                  <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                    Layout
                  </h3>
                  <div className="grid grid-cols-2 gap-3">
                    <label className="space-y-1">
                      <span className="text-xs text-muted-foreground">
                        Flow
                      </span>
                      <select
                        value={flow}
                        onChange={(event) =>
                          setFlow(event.target.value as ReaderFlow)
                        }
                        className="h-9 w-full rounded-md border border-border bg-background px-2"
                      >
                        <option value="paginated">Pages</option>
                        <option value="scrolled">Scroll</option>
                      </select>
                    </label>
                    <label className="space-y-1">
                      <span className="text-xs text-muted-foreground">
                        Writing
                      </span>
                      <select
                        value={writingMode}
                        onChange={(event) =>
                          setWritingMode(event.target.value as WritingMode)
                        }
                        className="h-9 w-full rounded-md border border-border bg-background px-2"
                      >
                        <option value="auto">Auto</option>
                        <option value="horizontal-tb">Horizontal</option>
                        <option value="vertical-rl">Vertical</option>
                      </select>
                    </label>
                  </div>
                  <div className="grid grid-cols-2 gap-2">
                    <Button
                      size="sm"
                      variant="secondary"
                      onClick={saveGlobalDefaults}
                    >
                      Save defaults
                    </Button>
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={loadGlobalDefaults}
                    >
                      Apply defaults
                    </Button>
                  </div>
                </section>

                <section className="space-y-3">
                  <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                    Typography
                  </h3>
                  <label className="space-y-1">
                    <span className="text-xs text-muted-foreground">Font</span>
                    <select
                      value={fontFamily}
                      onChange={(event) => setFontFamily(event.target.value)}
                      className="h-9 w-full rounded-md border border-border bg-background px-2"
                    >
                      <option value="Merriweather, Georgia, serif">
                        Merriweather (default)
                      </option>
                      <option value="Lora, Georgia, serif">Lora</option>
                      <option value="Georgia, serif">Georgia</option>
                      <option value="serif">System serif</option>
                      <option value="system-ui, sans-serif">
                        System sans-serif
                      </option>
                      <option value="Atkinson Hyperlegible, sans-serif">
                        Atkinson Hyperlegible
                      </option>
                      <option value="monospace">Monospace</option>
                      {(customFonts.data?.items ?? []).length > 0 && (
                        <optgroup label="Your fonts">
                          {(customFonts.data?.items ?? []).map((f) => (
                            <option
                              key={f.id}
                              value={JSON.stringify(f.name).slice(1, -1)}
                            >
                              {f.name}
                            </option>
                          ))}
                        </optgroup>
                      )}
                    </select>
                  </label>
                  <CustomFontUploader fonts={customFonts.data?.items ?? []} />
                  <label className="space-y-1">
                    <span className="text-xs text-muted-foreground">
                      Line height {lineHeight.toFixed(1)}
                    </span>
                    <input
                      type="range"
                      min="1.1"
                      max="2.4"
                      step="0.1"
                      value={lineHeight}
                      onChange={(event) =>
                        setLineHeight(Number(event.target.value))
                      }
                      className="w-full"
                    />
                  </label>
                  <label className="space-y-1">
                    <span className="text-xs text-muted-foreground">
                      Margin {margin}px
                    </span>
                    <input
                      type="range"
                      min="0"
                      max="88"
                      step="4"
                      value={margin}
                      onChange={(event) =>
                        setMargin(Number(event.target.value))
                      }
                      className="w-full"
                    />
                  </label>
                  <label className="space-y-1">
                    <span className="text-xs text-muted-foreground">
                      Max width {maxWidth}ch
                    </span>
                    <input
                      type="range"
                      min="42"
                      max="120"
                      step="2"
                      value={maxWidth}
                      onChange={(event) =>
                        setMaxWidth(Number(event.target.value))
                      }
                      className="w-full"
                    />
                  </label>
                  <div className="grid grid-cols-2 gap-3">
                    <label className="space-y-1">
                      <span className="text-xs text-muted-foreground">
                        Weight {fontWeight}
                      </span>
                      <input
                        type="range"
                        min="300"
                        max="800"
                        step="100"
                        value={fontWeight}
                        onChange={(event) =>
                          setFontWeight(Number(event.target.value))
                        }
                        className="w-full"
                      />
                    </label>
                    <label className="space-y-1">
                      <span className="text-xs text-muted-foreground">
                        Zoom {zoom}%
                      </span>
                      <input
                        type="range"
                        min="50"
                        max="200"
                        step="10"
                        value={zoom}
                        onChange={(event) =>
                          setZoom(Number(event.target.value))
                        }
                        className="w-full"
                      />
                    </label>
                  </div>
                  <div className="grid grid-cols-2 gap-3">
                    <label className="flex items-center gap-2 text-xs">
                      <input
                        type="checkbox"
                        checked={hyphenation}
                        onChange={(event) =>
                          setHyphenation(event.target.checked)
                        }
                      />
                      Hyphenation
                    </label>
                    <label className="flex items-center gap-2 text-xs">
                      <input
                        type="checkbox"
                        checked={rtl}
                        onChange={(event) => setRtl(event.target.checked)}
                      />
                      RTL
                    </label>
                  </div>
                </section>

                <section className="space-y-3 rounded-md border border-border bg-background p-3">
                  <h3 className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                    <Volume2 className="size-4" />
                    Read aloud
                  </h3>
                  <label className="space-y-1">
                    <span className="text-xs text-muted-foreground">
                      Rate {ttsRate.toFixed(1)}
                    </span>
                    <input
                      type="range"
                      min="0.5"
                      max="2.5"
                      step="0.1"
                      value={ttsRate}
                      onChange={(event) =>
                        setTtsRate(Number(event.target.value))
                      }
                      className="w-full"
                    />
                  </label>
                  <select
                    value={ttsVoice}
                    onChange={(event) => setTtsVoice(event.target.value)}
                    className="h-9 w-full rounded-md border border-border bg-background px-2"
                    aria-label="TTS voice"
                  >
                    <option value="">Default voice</option>
                    {ttsVoices.map((voice) => (
                      <option key={voice.voiceURI} value={voice.voiceURI}>
                        {voice.name}
                      </option>
                    ))}
                  </select>
                  <div className="grid grid-cols-4 gap-1">
                    <Button
                      size="icon"
                      variant="ghost"
                      onClick={() => jumpTtsChunk(-1)}
                      aria-label="Previous TTS chunk"
                    >
                      <SkipBack className="size-4" />
                    </Button>
                    <Button
                      size="icon"
                      variant="ghost"
                      onClick={pauseResumeTts}
                      disabled={!ttsActive}
                      aria-label="Pause or resume TTS"
                    >
                      {ttsPaused ? (
                        <Play className="size-4" />
                      ) : (
                        <Pause className="size-4" />
                      )}
                    </Button>
                    <Button
                      size="icon"
                      variant="ghost"
                      onClick={() => jumpTtsChunk(1)}
                      aria-label="Next TTS chunk"
                    >
                      <SkipForward className="size-4" />
                    </Button>
                    <Button
                      size="icon"
                      variant={ttsActive ? "secondary" : "ghost"}
                      onClick={toggleTts}
                      aria-label="Start or stop TTS"
                    >
                      {ttsActive ? (
                        <Square className="size-4" />
                      ) : (
                        <Play className="size-4" />
                      )}
                    </Button>
                  </div>
                  <label className="flex items-center gap-2">
                    <Timer className="size-4 text-muted-foreground" />
                    <select
                      value={ttsSleepMinutes}
                      onChange={(event) =>
                        setTtsSleepMinutes(Number(event.target.value))
                      }
                      className="h-9 flex-1 rounded-md border border-border bg-background px-2"
                      aria-label="TTS sleep timer"
                    >
                      <option value={0}>No timer</option>
                      <option value={5}>5 minutes</option>
                      <option value={15}>15 minutes</option>
                      <option value={30}>30 minutes</option>
                      <option value={60}>60 minutes</option>
                    </select>
                  </label>
                </section>

                <section className="space-y-3 rounded-md border border-border bg-background p-3">
                  <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                    Speed reading (RSVP)
                  </h3>
                  <label className="space-y-1">
                    <span className="text-xs text-muted-foreground">
                      {rsvpWpm} WPM
                    </span>
                    <input
                      type="range"
                      min="120"
                      max="700"
                      step="20"
                      value={rsvpWpm}
                      onChange={(event) =>
                        setRsvpWpm(Number(event.target.value))
                      }
                      className="w-full"
                    />
                  </label>
                  <Button
                    size="sm"
                    className="w-full"
                    variant={rsvpActive ? "secondary" : "default"}
                    onClick={() =>
                      rsvpActive ? setRsvpActive(false) : startRsvp()
                    }
                  >
                    {rsvpActive ? "Stop RSVP" : "Start RSVP"}
                  </Button>
                </section>

                <section className="space-y-3 rounded-md border border-border bg-background p-3">
                  <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                    Annotation backup
                  </h3>
                  <div className="grid grid-cols-2 gap-2">
                    <Button
                      size="sm"
                      variant="secondary"
                      onClick={exportAnnotationsJson}
                    >
                      Export JSON
                    </Button>
                    <label className="inline-flex h-8 cursor-pointer items-center justify-center rounded-md border border-border px-3 text-xs hover:bg-accent">
                      Import JSON
                      <input
                        type="file"
                        accept="application/json,.json"
                        className="hidden"
                        onChange={(event) => {
                          void importAnnotationsJson(event.target.files?.[0]);
                          event.currentTarget.value = "";
                        }}
                      />
                    </label>
                  </div>
                </section>

                <section className="space-y-3 rounded-md border border-border bg-background p-3">
                  <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                    KOReader bridge
                  </h3>
                  {externalProgress ? (
                    <div className="rounded border border-border px-2 py-1 text-xs text-muted-foreground">
                      Linked document {externalProgress.document}; latest{" "}
                      {Math.round((externalProgress.percentage ?? 0) * 100)}%
                      {externalProgress.canResume
                        ? " with CFI resume"
                        : " without CFI resume"}
                    </div>
                  ) : null}
                  <div className="flex gap-2">
                    <input
                      value={kosyncDocument}
                      onChange={(event) =>
                        setKosyncDocument(event.target.value)
                      }
                      placeholder="KOReader document id"
                      className="min-w-0 flex-1 rounded-md border border-border bg-background px-2 py-1.5 text-xs"
                    />
                    <Button
                      size="sm"
                      disabled={!kosyncDocument.trim()}
                      onClick={() => void saveKosyncLink()}
                    >
                      Link
                    </Button>
                  </div>
                </section>

                <section className="space-y-3 rounded-md border border-border bg-background p-3">
                  <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                    Diagnostics
                  </h3>
                  <div className="max-h-40 space-y-2 overflow-y-auto">
                    {diagnostics.length ? (
                      diagnostics.map((entry) => (
                        <div
                          key={`${entry.at}-${entry.message}`}
                          className="rounded border border-border px-2 py-1 text-xs"
                        >
                          <div className="flex items-center justify-between gap-2">
                            <span className="font-medium uppercase">
                              {entry.level}
                            </span>
                            <span className="text-muted-foreground">
                              {new Date(entry.at).toLocaleTimeString()}
                            </span>
                          </div>
                          <div className="mt-1 text-muted-foreground">
                            {entry.message}
                          </div>
                        </div>
                      ))
                    ) : (
                      <p className="text-xs text-muted-foreground">
                        No reader events yet.
                      </p>
                    )}
                  </div>
                </section>
              </div>
            ) : null}
          </aside>
        )}
      </div>
      {readerSelection ? (
        <div
          className="fixed z-50 flex max-w-[calc(100vw-1rem)] items-center gap-1 rounded-md border border-border bg-popover px-2 py-1.5 text-popover-foreground shadow-lg max-sm:inset-x-2 max-sm:bottom-[max(0.5rem,env(safe-area-inset-bottom))] max-sm:flex-wrap"
          style={toolbarStyle}
        >
          <Button
            size="icon"
            variant="ghost"
            onClick={() => void copySelection()}
            aria-label="Copy selection"
          >
            <Copy className="size-4" />
          </Button>
          <Button
            size="icon"
            variant="ghost"
            onClick={runQuickAction}
            aria-label="Run quick action"
          >
            {quickAction === "highlight" ? (
              <Highlighter className="size-4" />
            ) : quickAction === "note" ? (
              <Pencil className="size-4" />
            ) : quickAction === "search" ? (
              <Search className="size-4" />
            ) : (
              <Volume2 className="size-4" />
            )}
          </Button>
          <select
            value={quickAction}
            onChange={(event) =>
              setQuickAction(event.target.value as QuickAction)
            }
            className="h-8 rounded-md border border-border bg-background px-2 text-xs"
            aria-label="Selection quick action"
          >
            <option value="highlight">Highlight</option>
            <option value="note">Note</option>
            <option value="search">Search</option>
            <option value="speak">Speak</option>
          </select>
          <select
            value={highlightStyle}
            onChange={(event) =>
              setHighlightStyle(event.target.value as HighlightStyle)
            }
            className="h-8 rounded-md border border-border bg-background px-2 text-xs"
            aria-label="Highlight style"
          >
            {highlightStyles.map((style) => (
              <option key={style.value} value={style.value}>
                {style.label}
              </option>
            ))}
          </select>
          <div className="flex items-center gap-1 border-l border-border pl-1">
            {allHighlightColors.map((color) => (
              <button
                key={color.value}
                type="button"
                onClick={() => setHighlightColor(color.value)}
                className="size-6 rounded-full border border-border"
                style={{
                  backgroundColor: color.value,
                  outline:
                    highlightColor === color.value
                      ? "2px solid var(--primary)"
                      : "none",
                  outlineOffset: "2px",
                }}
                aria-label={`${color.label} highlight`}
              />
            ))}
            <input
              type="color"
              value={
                highlightColor.startsWith("#") ? highlightColor : "#facc15"
              }
              onChange={(event) => {
                const value = event.target.value;
                setHighlightColor(value);
                setCustomHighlightColors((current) =>
                  current.includes(value) ? current : [...current, value],
                );
              }}
              className="size-6 rounded border border-border bg-transparent p-0"
              aria-label="Custom highlight color"
            />
          </div>
          <Button
            size="icon"
            variant="ghost"
            onClick={startNoteFromSelection}
            aria-label="Annotate selection"
          >
            <Pencil className="size-4" />
          </Button>
          <Button
            size="icon"
            variant="ghost"
            onClick={() => void searchSelection()}
            aria-label="Search selection"
          >
            <Search className="size-4" />
          </Button>
          <Button
            size="icon"
            variant="ghost"
            onClick={speakSelection}
            aria-label="Read selection aloud"
          >
            <Volume2 className="size-4" />
          </Button>
          <Button
            size="icon"
            variant="ghost"
            onClick={() => defineSelection()}
            aria-label="Define selected word"
          >
            <BookOpen className="size-4" />
          </Button>
          <Button
            size="icon"
            variant="ghost"
            onClick={() => translateSelection()}
            aria-label="Translate selection"
          >
            <Languages className="size-4" />
          </Button>
        </div>
      ) : null}
      {definePopover ? (
        <DefinePopover
          word={definePopover.word}
          onClose={() => setDefinePopover(null)}
        />
      ) : null}
      {translatePopover ? (
        <TranslatePopover
          text={translatePopover.text}
          onClose={() => setTranslatePopover(null)}
        />
      ) : null}
      {readingRuler ? (
        <div
          role="separator"
          aria-label="Reading ruler — drag vertically to reposition"
          aria-orientation="horizontal"
          className="fixed left-0 right-0 z-30 -translate-y-1/2 cursor-ns-resize touch-none select-none border-y border-yellow-400/60 bg-yellow-200/20"
          style={{
            top: `${rulerTop}%`,
            // One line of body text ≈ 16px * fontSize% * lineHeight; add a
            // few px of padding so the band visually frames the line.
            height: `${Math.min(96, Math.max(28, Math.round(16 * (fontSize / 100) * lineHeight) + 6))}px`,
          }}
          onPointerDown={(event) => {
            event.preventDefault();
            const bandRect = event.currentTarget.getBoundingClientRect();
            const offsetY = event.clientY - (bandRect.top + bandRect.height / 2);
            rulerDragRef.current = { offsetY };
            event.currentTarget.setPointerCapture(event.pointerId);
          }}
          onPointerMove={(event) => {
            if (!rulerDragRef.current) return;
            const offsetY = rulerDragRef.current.offsetY;
            const next =
              ((event.clientY - offsetY) / window.innerHeight) * 100;
            setRulerTop(Math.min(100, Math.max(0, next)));
          }}
          onPointerUp={(event) => {
            rulerDragRef.current = null;
            event.currentTarget.releasePointerCapture(event.pointerId);
          }}
          onPointerCancel={() => {
            rulerDragRef.current = null;
          }}
        />
      ) : null}
      {rsvpActive ? (
        <div className="fixed inset-x-4 top-24 z-50 mx-auto flex max-w-xl items-center justify-between rounded-md border border-border bg-popover px-4 py-3 text-popover-foreground shadow-lg">
          <Button
            size="icon"
            variant="ghost"
            onClick={() => setRsvpIndex((value) => Math.max(0, value - 1))}
            aria-label="Previous RSVP word"
          >
            <SkipBack className="size-4" />
          </Button>
          <div className="min-w-0 flex-1 px-4 text-center">
            <div className="truncate text-3xl font-semibold">
              {rsvpWords[rsvpIndex]}
            </div>
            <div className="mt-1 text-xs text-muted-foreground">
              {rsvpIndex + 1} / {rsvpWords.length}
            </div>
          </div>
          <Button
            size="icon"
            variant="ghost"
            onClick={() =>
              setRsvpIndex((value) => Math.min(rsvpWords.length - 1, value + 1))
            }
            aria-label="Next RSVP word"
          >
            <SkipForward className="size-4" />
          </Button>
          <Button
            size="icon"
            variant="ghost"
            onClick={() => setRsvpActive(false)}
            aria-label="Stop RSVP"
          >
            <Square className="size-4" />
          </Button>
        </div>
      ) : null}
      {ttsActive && ttsChunks[ttsChunkIndex] ? (
        <div className="fixed inset-x-4 bottom-6 z-40 mx-auto max-w-2xl rounded-md border border-border bg-popover px-4 py-3 text-popover-foreground shadow-lg">
          <div className="mb-2 flex items-center justify-between gap-3 text-xs text-muted-foreground">
            <span>
              Reading {ttsChunkIndex + 1} / {ttsChunks.length}
            </span>
            <div className="flex gap-1">
              <Button
                size="icon"
                variant="ghost"
                onClick={() => jumpTtsChunk(-1)}
                aria-label="Previous spoken chunk"
              >
                <SkipBack className="size-4" />
              </Button>
              <Button
                size="icon"
                variant="ghost"
                onClick={pauseResumeTts}
                aria-label="Pause spoken chunk"
              >
                {ttsPaused ? (
                  <Play className="size-4" />
                ) : (
                  <Pause className="size-4" />
                )}
              </Button>
              <Button
                size="icon"
                variant="ghost"
                onClick={() => jumpTtsChunk(1)}
                aria-label="Next spoken chunk"
              >
                <SkipForward className="size-4" />
              </Button>
              <Button
                size="icon"
                variant="ghost"
                onClick={toggleTts}
                aria-label="Stop spoken chunk"
              >
                <Square className="size-4" />
              </Button>
            </div>
          </div>
          <p className="line-clamp-3 text-sm leading-6">
            {ttsChunks[ttsChunkIndex]}
          </p>
        </div>
      ) : null}
      {contentPopup ? (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
          <div className="max-h-[85vh] w-full max-w-3xl overflow-auto rounded-md border border-border bg-popover p-4 text-popover-foreground shadow-xl">
            <div className="mb-3 flex items-center justify-between gap-3">
              <div className="min-w-0 truncate text-sm font-medium">
                {contentPopup.title}
              </div>
              <Button
                size="sm"
                variant="ghost"
                onClick={() => setContentPopup(null)}
              >
                Close
              </Button>
            </div>
            {contentPopup.kind === "image" && contentPopup.src ? (
              <img
                src={contentPopup.src}
                alt={contentPopup.title}
                className="mx-auto max-h-[70vh] max-w-full object-contain"
              />
            ) : null}
            {contentPopup.kind === "table" && contentPopup.html ? (
              <div
                className="overflow-auto text-sm [&_table]:w-full [&_td]:border [&_td]:border-border [&_td]:p-2 [&_th]:border [&_th]:border-border [&_th]:p-2"
                dangerouslySetInnerHTML={{ __html: contentPopup.html }}
              />
            ) : null}
            {contentPopup.kind === "footnote" ? (
              <p className="whitespace-pre-wrap text-sm leading-6">
                {contentPopup.content}
              </p>
            ) : null}
          </div>
        </div>
      ) : null}
      <div className="hidden">book-id:{id ? "loaded" : "pending"}</div>
    </div>
  );
}
