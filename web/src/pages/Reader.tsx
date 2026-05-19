import { useCallback, useEffect, useRef, useState } from "react";
import { Link, useParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
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
  Focus,
  Highlighter,
  ListTree,
  Maximize2,
  Moon,
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
  mountPath,
  putReaderConfig,
  updateAnnotation,
  type Annotation,
  type ExternalReaderProgress,
} from "@/lib/api";
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

type ReaderPanelTab = "toc" | "search" | "notes" | "bookmarks" | "settings";
type HighlightStyle = "highlight" | "underline" | "squiggly";
type ReaderTheme = "light" | "sepia" | "dark";
type ReaderFlow = "paginated" | "scrolled";
type WritingMode = "auto" | "horizontal-tb" | "vertical-rl";
type QuickAction = "highlight" | "note" | "search" | "speak";

const highlightColors = [
  { label: "Yellow", value: "#facc15" },
  { label: "Red", value: "#f87171" },
  { label: "Green", value: "#86efac" },
  { label: "Blue", value: "#93c5fd" },
  { label: "Violet", value: "#c4b5fd" },
];

const highlightStyles: Array<{ label: string; value: HighlightStyle }> = [
  { label: "Highlight", value: "highlight" },
  { label: "Underline", value: "underline" },
  { label: "Squiggle", value: "squiggly" },
];

const globalReaderDefaultsKey = "continuum-ebooks-reader-defaults";

export default function Reader() {
  const params = useParams();
  const id = params.id ?? "";
  const readerRef = useRef<ReadestLiteReaderHandle | null>(null);
  const [pct, setPct] = useState(0);
  const [scrubPct, setScrubPct] = useState(0);
  const [currentSection, setCurrentSection] = useState("");
  const [fontSize, setFontSize] = useState(100);
  const [theme, setTheme] = useState<ReaderTheme>("light");
  const [spread, setSpread] = useState<"auto" | "none">("auto");
  const [flow, setFlow] = useState<ReaderFlow>("paginated");
  const [fontFamily, setFontFamily] = useState("serif");
  const [fontWeight, setFontWeight] = useState(400);
  const [hyphenation, setHyphenation] = useState(true);
  const [lineHeight, setLineHeight] = useState(1.6);
  const [margin, setMargin] = useState(24);
  const [maxWidth, setMaxWidth] = useState(72);
  const [paragraphFocus, setParagraphFocus] = useState(false);
  const [readingRuler, setReadingRuler] = useState(false);
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
  const [highlightColor, setHighlightColor] = useState("#facc15");
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

  useEffect(() => {
    const files = detail.data?.files ?? [];
    if (!files.length) return;
    const preferred =
      files.find((file) => file.format.toLowerCase() === "epub") ??
      files.find((file) =>
        ["pdf", "cbz", "cbr"].includes(file.format.toLowerCase()),
      ) ??
      files[0];
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
        if (typeof viewSettings.paragraphFocus === "boolean") {
          setParagraphFocus(viewSettings.paragraphFocus);
        }
        if (typeof viewSettings.quickAction === "string") {
          setQuickAction(viewSettings.quickAction as QuickAction);
        }
        if (typeof viewSettings.readingRuler === "boolean") {
          setReadingRuler(viewSettings.readingRuler);
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

  useEffect(() => {
    if (!id || !settingsLoaded) return;
    const timeout = window.setTimeout(() => {
      void getReaderConfig(id).then((envelope) => {
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
            fontFamily,
            fontSize,
            fontWeight,
            customHighlightColors,
            hyphenation,
            lineHeight,
            margin,
            maxWidth,
            paragraphFocus,
            quickAction,
            readingRuler,
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
      });
    }, 500);
    return () => window.clearTimeout(timeout);
  }, [
    flow,
    fontFamily,
    fontSize,
    fontWeight,
    customHighlightColors,
    hyphenation,
    id,
    lineHeight,
    margin,
    maxWidth,
    paragraphFocus,
    quickAction,
    readingRuler,
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
  }, []);

  const fileURL = `${mountPath()}/api/v1/me/books/${encodeURIComponent(id)}/file?format=${encodeURIComponent(selectedFormat)}`;
  const normalizedFormat = selectedFormat.toLowerCase();
  const unsupportedInlineFormats = new Set(["rar"]);
  const canUseReader =
    !!selectedFormat && !unsupportedInlineFormats.has(normalizedFormat);
  const readerMode =
    normalizedFormat === "pdf"
      ? "PDF"
      : ["cbz", "cbr", "zip"].includes(normalizedFormat)
        ? "Comic"
        : "Book";

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
    fontFamily,
    fontSize,
    fontWeight,
    customHighlightColors,
    hyphenation,
    lineHeight,
    margin,
    maxWidth,
    paragraphFocus,
    quickAction,
    readingRuler,
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
    if (typeof defaults.paragraphFocus === "boolean")
      setParagraphFocus(defaults.paragraphFocus);
    if (typeof defaults.quickAction === "string")
      setQuickAction(defaults.quickAction as QuickAction);
    if (typeof defaults.readingRuler === "boolean")
      setReadingRuler(defaults.readingRuler);
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
  const allHighlightColors = [
    ...highlightColors,
    ...customHighlightColors.map((value) => ({ label: value, value })),
  ];

  const toolbarStyle = readerSelection
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
              variant={theme === "dark" ? "secondary" : "ghost"}
              onClick={() => setTheme(theme === "dark" ? "light" : "dark")}
              aria-label="Toggle dark reader theme"
            >
              {theme === "dark" ? (
                <Sun className="size-4" />
              ) : (
                <Moon className="size-4" />
              )}
            </Button>
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
            >
              <Ruler className="size-4" />
            </Button>
            <Button
              size="icon"
              variant={paragraphFocus ? "secondary" : "ghost"}
              onClick={() => setParagraphFocus((value) => !value)}
              aria-label="Toggle paragraph focus"
            >
              <Focus className="size-4" />
            </Button>
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
              variant={ttsActive ? "secondary" : "ghost"}
              onClick={toggleTts}
              aria-label={ttsActive ? "Stop reading aloud" : "Read aloud"}
            >
              {ttsActive ? (
                <Square className="size-4" />
              ) : (
                <Volume2 className="size-4" />
              )}
            </Button>
            <Button
              size="icon"
              variant={rsvpActive ? "secondary" : "ghost"}
              onClick={() => (rsvpActive ? setRsvpActive(false) : startRsvp())}
              aria-label="Toggle RSVP reading"
            >
              {rsvpActive ? (
                <Square className="size-4" />
              ) : (
                <Play className="size-4" />
              )}
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
      {(externalProgress || readerMode !== "Book" || readaloudAvailable) && (
        <div className="flex flex-wrap items-center gap-2 border-b border-border bg-muted/40 px-4 py-2 text-xs text-muted-foreground">
          <span className="font-medium text-foreground">{readerMode} mode</span>
          {readerMode === "PDF" ? (
            <span>
              PDF progress and annotations use the Foliate location model when
              available.
            </span>
          ) : null}
          {readerMode === "Comic" ? (
            <span>
              Comic archives render inline with page navigation and saved
              progress.
            </span>
          ) : null}
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
            settings={{
              flow,
              fontFamily,
              fontSize,
              fontWeight,
              hyphenation,
              lineHeight,
              margin,
              maxWidth,
              paragraphFocus,
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
          <div className="flex h-full flex-col items-center justify-center gap-3 p-6 text-center">
            <div className="text-lg font-semibold">
              {selectedFormat.toUpperCase()} ready
            </div>
            <p className="max-w-md text-sm text-muted-foreground">
              Browser-native rendering for this archive format is not available
              yet. Open or download the file to read it in a comic reader.
            </p>
            <Button asChild>
              <a href={fileURL}>Open file</a>
            </Button>
          </div>
        )}
        {showPanel && canUseReader && (
          <aside className="min-h-0 overflow-y-auto border-l border-border bg-card p-4">
            <div className="grid grid-cols-5 gap-1 rounded-md border border-border bg-background p-1">
              <Button
                size="sm"
                variant={panelTab === "toc" ? "secondary" : "ghost"}
                onClick={() => setPanelTab("toc")}
              >
                <ListTree className="mr-1 size-4" />
                TOC
              </Button>
              <Button
                size="sm"
                variant={panelTab === "search" ? "secondary" : "ghost"}
                onClick={() => setPanelTab("search")}
              >
                <Search className="mr-1 size-4" />
                Find
              </Button>
              <Button
                size="sm"
                variant={panelTab === "notes" ? "secondary" : "ghost"}
                onClick={() => setPanelTab("notes")}
              >
                <NotebookPen className="mr-1 size-4" />
                Notes
              </Button>
              <Button
                size="sm"
                variant={panelTab === "bookmarks" ? "secondary" : "ghost"}
                onClick={() => setPanelTab("bookmarks")}
              >
                <Bookmark className="mr-1 size-4" />
                Marks
              </Button>
              <Button
                size="sm"
                variant={panelTab === "settings" ? "secondary" : "ghost"}
                onClick={() => setPanelTab("settings")}
              >
                <Settings className="mr-1 size-4" />
                Set
              </Button>
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
                      <div className="line-clamp-3">
                        {result.excerpt?.pre}
                        <mark className="bg-yellow-200 px-0.5 text-foreground">
                          {result.excerpt?.match}
                        </mark>
                        {result.excerpt?.post || result.cfi}
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
                <div className="mt-4 space-y-3">
                  {filteredNoteItems.map((annotation) => (
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
                          {annotation.kind === "note" ? "Note" : "Highlight"}
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
              <div className="mt-4 space-y-5 text-sm">
                <div className="grid grid-cols-2 gap-2">
                  <label className="space-y-1">
                    <span className="text-xs text-muted-foreground">Flow</span>
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
                    variant="ghost"
                    onClick={loadGlobalDefaults}
                  >
                    Apply defaults
                  </Button>
                </div>
                <label className="space-y-1">
                  <span className="text-xs text-muted-foreground">Font</span>
                  <select
                    value={fontFamily}
                    onChange={(event) => setFontFamily(event.target.value)}
                    className="h-9 w-full rounded-md border border-border bg-background px-2"
                  >
                    <option value="serif">Serif</option>
                    <option value="sans-serif">Sans serif</option>
                    <option value="monospace">Monospace</option>
                    <option value="Georgia, serif">Georgia</option>
                    <option value="Atkinson Hyperlegible, sans-serif">
                      Atkinson
                    </option>
                  </select>
                </label>
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
                    onChange={(event) => setMargin(Number(event.target.value))}
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
                <div className="grid grid-cols-2 gap-2">
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
                      onChange={(event) => setZoom(Number(event.target.value))}
                      className="w-full"
                    />
                  </label>
                </div>
                <div className="grid grid-cols-2 gap-2">
                  <label className="flex items-center gap-2">
                    <input
                      type="checkbox"
                      checked={hyphenation}
                      onChange={(event) => setHyphenation(event.target.checked)}
                    />
                    Hyphenation
                  </label>
                  <label className="flex items-center gap-2">
                    <input
                      type="checkbox"
                      checked={rtl}
                      onChange={(event) => setRtl(event.target.checked)}
                    />
                    RTL
                  </label>
                </div>
                <div className="rounded-md border border-border bg-background p-3">
                  <div className="mb-2 flex items-center gap-2 text-xs font-medium text-muted-foreground">
                    <Volume2 className="size-4" />
                    TTS
                  </div>
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
                    className="mt-2 h-9 w-full rounded-md border border-border bg-background px-2"
                    aria-label="TTS voice"
                  >
                    <option value="">Default voice</option>
                    {ttsVoices.map((voice) => (
                      <option key={voice.voiceURI} value={voice.voiceURI}>
                        {voice.name}
                      </option>
                    ))}
                  </select>
                  <div className="mt-2 grid grid-cols-4 gap-1">
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
                      variant="ghost"
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
                  <label className="mt-2 flex items-center gap-2">
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
                </div>
                <div className="rounded-md border border-border bg-background p-3">
                  <div className="mb-2 text-xs font-medium text-muted-foreground">
                    RSVP speed {rsvpWpm} WPM
                  </div>
                  <input
                    type="range"
                    min="120"
                    max="700"
                    step="20"
                    value={rsvpWpm}
                    onChange={(event) => setRsvpWpm(Number(event.target.value))}
                    className="w-full"
                  />
                  <Button
                    size="sm"
                    className="mt-2 w-full"
                    variant={rsvpActive ? "secondary" : "default"}
                    onClick={() =>
                      rsvpActive ? setRsvpActive(false) : startRsvp()
                    }
                  >
                    {rsvpActive ? "Stop RSVP" : "Start RSVP"}
                  </Button>
                </div>
                <div className="rounded-md border border-border bg-background p-3">
                  <div className="mb-2 text-xs font-medium text-muted-foreground">
                    Annotation backup
                  </div>
                  <div className="grid grid-cols-2 gap-2">
                    <Button
                      size="sm"
                      variant="secondary"
                      onClick={exportAnnotationsJson}
                    >
                      Export JSON
                    </Button>
                    <label className="inline-flex h-9 cursor-pointer items-center justify-center rounded-md border border-border px-2 text-xs hover:bg-accent">
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
                </div>
                <div className="rounded-md border border-border bg-background p-3">
                  <div className="mb-2 text-xs font-medium text-muted-foreground">
                    Diagnostics
                  </div>
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
                </div>
                <div className="rounded-md border border-border bg-background p-3">
                  <div className="mb-2 text-xs font-medium text-muted-foreground">
                    KOReader bridge
                  </div>
                  {externalProgress ? (
                    <div className="mb-3 rounded border border-border px-2 py-1 text-xs text-muted-foreground">
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
                </div>
              </div>
            ) : null}
          </aside>
        )}
      </div>
      {readerSelection ? (
        <div
          className="fixed z-50 flex max-w-[calc(100vw-1rem)] items-center gap-1 rounded-md border border-border bg-popover px-2 py-1.5 text-popover-foreground shadow-lg"
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
        </div>
      ) : null}
      {readingRuler ? (
        <div
          className="pointer-events-none fixed left-0 right-0 top-1/2 z-30 h-12 -translate-y-1/2 border-y border-yellow-400/60 bg-yellow-200/20"
          aria-hidden="true"
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
