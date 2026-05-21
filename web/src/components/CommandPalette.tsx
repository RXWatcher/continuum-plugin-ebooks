import {
  createContext,
  ReactNode,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { useNavigate } from "react-router";
import { useQuery } from "@tanstack/react-query";
import {
  ArrowRight,
  BookOpen,
  Bookmark,
  Compass,
  Library,
  Search,
  Send,
  Settings,
  Smartphone,
  Tag,
  Users,
} from "lucide-react";
import { listCatalog } from "@/lib/api";
import { cn } from "@/lib/utils";

// Cmd-K command palette for the ebooks SPA. Ported from the
// audiobooks plugin's CommandPalette — same scoring logic, same
// UX, adapted nav targets + a recent-books fetch backed by
// listCatalog.

type Cmd = {
  id: string;
  label: string;
  hint?: string;
  icon: ReactNode;
  perform: () => void;
};

function score(query: string, candidate: string): number {
  if (!query) return 1;
  const q = query.toLowerCase();
  const c = candidate.toLowerCase();
  if (q === c) return 1000;
  if (c.startsWith(q)) return 500;
  let s = 0;
  let lastIdx = -1;
  let consec = 0;
  for (let i = 0; i < q.length; i++) {
    const idx = c.indexOf(q[i], lastIdx + 1);
    if (idx < 0) return 0;
    if (idx === lastIdx + 1) {
      consec++;
      s += 5 + consec;
    } else {
      consec = 0;
      s += 1;
    }
    if (idx === 0 || c[idx - 1] === " ") s += 3;
    lastIdx = idx;
  }
  return s;
}

type Ctx = { open: () => void; close: () => void; isOpen: boolean };
const CommandCtx = createContext<Ctx | null>(null);

export function useCommandPalette() {
  const ctx = useContext(CommandCtx);
  if (!ctx) throw new Error("useCommandPalette must be used within CommandPaletteProvider");
  return ctx;
}

export function CommandPaletteProvider({ children }: { children: ReactNode }) {
  const [isOpen, setOpen] = useState(false);
  const open = useCallback(() => setOpen(true), []);
  const close = useCallback(() => setOpen(false), []);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setOpen((v) => !v);
      } else if (e.key === "Escape") {
        setOpen(false);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  return (
    <CommandCtx.Provider value={{ open, close, isOpen }}>
      {children}
      {isOpen && <Palette close={close} />}
    </CommandCtx.Provider>
  );
}

function Palette({ close }: { close: () => void }) {
  const navigate = useNavigate();
  const [query, setQuery] = useState("");
  const [selectedIdx, setSelectedIdx] = useState(0);
  const inputRef = useRef<HTMLInputElement | null>(null);

  const recents = useQuery({
    queryKey: ["cmdk-recent-books"],
    queryFn: () => listCatalog("", "added", "desc", 20),
    staleTime: 60_000,
  });

  const commands = useMemo<Cmd[]>(() => {
    const navCommands: Cmd[] = [
      { id: "nav-home", label: "Home", icon: <Compass className="size-4" />, perform: () => navigate("/") },
      { id: "nav-library", label: "Library", icon: <Library className="size-4" />, perform: () => navigate("/library") },
      { id: "nav-authors", label: "Authors", icon: <Users className="size-4" />, perform: () => navigate("/authors") },
      { id: "nav-series", label: "Series", icon: <BookOpen className="size-4" />, perform: () => navigate("/series") },
      { id: "nav-genres", label: "Genres", icon: <Tag className="size-4" />, perform: () => navigate("/genres") },
      { id: "nav-collections", label: "Collections", icon: <Bookmark className="size-4" />, perform: () => navigate("/collections") },
      { id: "nav-requests", label: "My Requests", icon: <Send className="size-4" />, perform: () => navigate("/me/requests") },
      { id: "nav-apps", label: "Apps", icon: <Smartphone className="size-4" />, perform: () => navigate("/apps") },
      { id: "nav-admin", label: "Admin", hint: "requires admin access", icon: <Settings className="size-4" />, perform: () => navigate("/admin") },
    ];
    const bookCommands: Cmd[] = (recents.data?.items ?? []).map((b) => ({
      id: `book-${b.id}`,
      label: b.title,
      hint: b.authors?.join(", "),
      icon: <BookOpen className="size-4" />,
      perform: () => navigate(`/library/${encodeURIComponent(b.id)}`),
    }));
    return [...navCommands, ...bookCommands];
  }, [recents.data, navigate]);

  const filtered = useMemo(() => {
    if (!query.trim()) return commands.slice(0, 20);
    const scored = commands
      .map((c) => ({ c, s: Math.max(score(query, c.label), score(query, c.hint ?? "") / 2) }))
      .filter((x) => x.s > 0)
      .sort((a, b) => b.s - a.s);
    return scored.slice(0, 20).map((x) => x.c);
  }, [commands, query]);

  useEffect(() => {
    setSelectedIdx(0);
  }, [query]);

  useEffect(() => {
    inputRef.current?.focus();
  }, []);

  const perform = useCallback(
    (cmd: Cmd) => {
      close();
      cmd.perform();
    },
    [close],
  );

  const onKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      setSelectedIdx((i) => Math.min(filtered.length - 1, i + 1));
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setSelectedIdx((i) => Math.max(0, i - 1));
    } else if (e.key === "Enter" && filtered[selectedIdx]) {
      e.preventDefault();
      perform(filtered[selectedIdx]);
    }
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center bg-black/60 backdrop-blur-sm pt-[15vh]"
      onClick={close}
    >
      <div
        className="bg-surface border-border w-full max-w-xl overflow-hidden rounded-xl border shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="border-border flex items-center gap-2 border-b px-4 py-3">
          <Search className="text-muted-foreground size-4" />
          <input
            ref={inputRef}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={onKeyDown}
            placeholder="Search books, libraries, or actions…"
            className="flex-1 bg-transparent text-base outline-none"
          />
          <kbd className="border-border text-muted-foreground hidden rounded border px-1.5 py-0.5 text-xs sm:inline-block">
            esc
          </kbd>
        </div>
        <div className="max-h-[60vh] overflow-y-auto">
          {filtered.length === 0 ? (
            <div className="text-muted-foreground px-4 py-6 text-center text-sm">No matches.</div>
          ) : (
            filtered.map((c, i) => (
              <button
                key={c.id}
                type="button"
                onClick={() => perform(c)}
                onMouseEnter={() => setSelectedIdx(i)}
                className={cn(
                  "flex w-full items-center gap-3 px-4 py-2.5 text-left text-sm",
                  i === selectedIdx ? "bg-surface-hover" : "hover:bg-surface-hover/60",
                )}
              >
                <span className="text-muted-foreground shrink-0">{c.icon}</span>
                <span className="min-w-0 flex-1">
                  <span className="block truncate">{c.label}</span>
                  {c.hint && (
                    <span className="text-muted-foreground block truncate text-xs">{c.hint}</span>
                  )}
                </span>
                <ArrowRight className="text-muted-foreground/60 size-3 shrink-0" />
              </button>
            ))
          )}
        </div>
        <div className="border-border text-muted-foreground flex items-center justify-between border-t px-4 py-2 text-xs">
          <span>
            <kbd className="border-border rounded border px-1">↑</kbd>{" "}
            <kbd className="border-border rounded border px-1">↓</kbd> navigate
          </span>
          <span>
            <kbd className="border-border rounded border px-1">↵</kbd> open
          </span>
        </div>
      </div>
    </div>
  );
}
