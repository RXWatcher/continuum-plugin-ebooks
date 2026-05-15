import { useEffect, useRef, useState } from "react";
import { Link, useParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import ePub, { type Book, type Rendition } from "epubjs";
import {
  ArrowLeft,
  ArrowRight,
  ChevronLeft,
  Columns2,
  Moon,
  Sun,
  Type,
} from "lucide-react";
import {
  createAnnotation,
  getBook,
  getBookUserData,
  listAnnotations,
  mountPath,
  updateProgress,
} from "@/lib/api";
import { Button } from "@/components/ui/button";

export default function Reader() {
  const params = useParams();
  const id = params.id ?? "";
  const viewerRef = useRef<HTMLDivElement>(null);
  const renditionRef = useRef<Rendition | null>(null);
  const [book, setBook] = useState<Book | null>(null);
  const [pct, setPct] = useState(0);
  const [fontSize, setFontSize] = useState(100);
  const [theme, setTheme] = useState<"light" | "sepia" | "dark">("light");
  const [spread, setSpread] = useState<"auto" | "none">("auto");
  const [selectedFormat, setSelectedFormat] = useState("epub");
  const [currentCfi, setCurrentCfi] = useState("");
  const [showNotes, setShowNotes] = useState(false);
  const [noteText, setNoteText] = useState("");

  const detail = useQuery({
    queryKey: ["book", id],
    queryFn: () => getBook(id),
    enabled: !!id,
  });
  const userData = useQuery({
    queryKey: ["book-user-data", id],
    queryFn: () => getBookUserData(id),
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
    if (!id || !viewerRef.current || selectedFormat !== "epub") return;
    const url = `${mountPath()}/api/v1/me/books/${encodeURIComponent(id)}/file?format=${encodeURIComponent(selectedFormat)}`;
    const b = ePub(url);
    setBook(b);
    const r = b.renderTo(viewerRef.current, {
      width: "100%",
      height: "100%",
      spread,
    });
    renditionRef.current = r;
    r.display(userData.data?.last_cfi || undefined);

    r.on(
      "relocated",
      (location: { start?: { cfi?: string; percentage?: number } }) => {
        const cfi = location.start?.cfi ?? "";
        const percentage = location.start?.percentage ?? 0;
        setPct(Math.round(percentage * 100));
        setCurrentCfi(cfi);
        void updateProgress(id, {
          last_cfi: cfi,
          read_progress: percentage,
          is_finished: percentage >= 0.98,
        }).catch(() => {});
      },
    );
    return () => {
      r.destroy();
      b.destroy();
      renditionRef.current = null;
    };
  }, [id, selectedFormat, spread, userData.data?.last_cfi]);

  useEffect(() => {
    const r = renditionRef.current;
    if (!r) return;
    r.themes.fontSize(`${fontSize}%`);
    r.themes.register("continuum-light", {
      body: { background: "#ffffff", color: "#171717" },
      a: { color: "#2563eb" },
    });
    r.themes.register("continuum-sepia", {
      body: { background: "#f4ecd8", color: "#1f1b16" },
      a: { color: "#8a4b12" },
    });
    r.themes.register("continuum-dark", {
      body: { background: "#111111", color: "#f5f5f5" },
      a: { color: "#93c5fd" },
    });
    r.themes.select(`continuum-${theme}`);
  }, [fontSize, theme, book]);

  const fileURL = `${mountPath()}/api/v1/me/books/${encodeURIComponent(id)}/file?format=${encodeURIComponent(selectedFormat)}`;
  const isEpub = selectedFormat === "epub";

  const saveNote = async () => {
    if (!noteText.trim() || !currentCfi) return;
    await createAnnotation(id, {
      cfi_range: currentCfi,
      kind: "note",
      color: "#facc15",
      note_text: noteText.trim(),
    });
    setNoteText("");
    await annotations.refetch();
  };

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
            {(detail.data?.authors ?? []).join(", ")}
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
        {isEpub && (
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
              size="sm"
              variant={showNotes ? "secondary" : "ghost"}
              onClick={() => setShowNotes((value) => !value)}
            >
              Notes
            </Button>
          </>
        )}
        <div className="flex-1" />
        <span className="text-xs text-muted-foreground">{pct}%</span>
        {isEpub && (
          <>
            <Button
              size="icon"
              variant="ghost"
              onClick={() => renditionRef.current?.prev()}
              aria-label="Previous"
            >
              <ArrowLeft className="size-4" />
            </Button>
            <Button
              size="icon"
              variant="ghost"
              onClick={() => renditionRef.current?.next()}
              aria-label="Next"
            >
              <ArrowRight className="size-4" />
            </Button>
          </>
        )}
      </header>
      <div className="grid min-h-0 flex-1 grid-cols-1 lg:grid-cols-[1fr_20rem]">
        {isEpub ? (
          <div
            ref={viewerRef}
            className="min-h-0 overflow-hidden bg-background"
          />
        ) : selectedFormat === "pdf" ? (
          <iframe
            title={detail.data?.title || "Reader"}
            src={fileURL}
            className="h-full w-full border-0"
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
        {showNotes && isEpub && (
          <aside className="min-h-0 overflow-y-auto border-l border-border bg-card p-4">
            <h2 className="text-sm font-semibold">Notes</h2>
            <div className="mt-3 space-y-2">
              <textarea
                value={noteText}
                onChange={(e) => setNoteText(e.target.value)}
                placeholder="Add a note at the current reading position"
                className="min-h-24 w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
              />
              <Button
                size="sm"
                disabled={!noteText.trim() || !currentCfi}
                onClick={() => void saveNote()}
              >
                Save note
              </Button>
            </div>
            <div className="mt-4 space-y-3">
              {(annotations.data?.items ?? []).map((annotation) => (
                <button
                  key={annotation.id}
                  type="button"
                  onClick={() => {
                    if (annotation.cfi_range && renditionRef.current) {
                      renditionRef.current.display(annotation.cfi_range);
                    }
                  }}
                  className="block w-full rounded-md border border-border bg-background p-3 text-left text-sm hover:bg-accent"
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
              ))}
            </div>
          </aside>
        )}
      </div>
      <div className="hidden">book-id:{book ? "loaded" : "pending"}</div>
    </div>
  );
}
