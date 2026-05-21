import { useQuery } from "@tanstack/react-query";
import { X } from "lucide-react";
import { useEffect } from "react";
import { api } from "@/lib/api";
import { Card } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";

// DefinePopover shows Wiktionary definitions for a single word.
// Server proxies the Wiktionary REST API + flattens to a list of
// {part_of_speech, definition, example} entries.

type LookupResult = {
  word: string;
  language: string;
  entries?: { part_of_speech: string; definition: string; example?: string }[];
};

export default function DefinePopover({
  word,
  onClose,
}: {
  word: string;
  onClose: () => void;
}) {
  const def = useQuery({
    queryKey: ["define", word],
    queryFn: async () => {
      const res = await api.fetchRaw(
        `/dictionary/lookup?word=${encodeURIComponent(word)}`,
      );
      if (!res.ok) {
        // authedFetch doesn't throw on 4xx; the dictionary route
        // returns text/plain bodies on error. Surface the body so
        // the popover shows a real message instead of a JSON parse
        // error.
        const text = await res.text().catch(() => res.statusText);
        throw new Error(text || `lookup failed (${res.status})`);
      }
      return (await res.json()) as LookupResult;
    },
    enabled: !!word,
  });

  // Esc dismisses; outside-click handled by the parent overlay.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  const entries = def.data?.entries ?? [];

  return (
    <PopoverShell title={word} onClose={onClose}>
      {def.isLoading ? (
        <Skeleton className="h-24 w-full" />
      ) : def.isError ? (
        <p className="text-destructive text-sm">
          {def.error instanceof Error ? def.error.message : "Lookup failed"}
        </p>
      ) : entries.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          No definition found in Wiktionary.
        </p>
      ) : (
        <ol className="space-y-3">
          {entries.slice(0, 6).map((entry, i) => (
            <li key={i}>
              <div className="text-muted-foreground mb-1 text-xs uppercase tracking-wide">
                {entry.part_of_speech}
              </div>
              <div className="text-sm">{entry.definition}</div>
              {entry.example && (
                <div className="text-muted-foreground mt-1 text-xs italic">
                  "{entry.example}"
                </div>
              )}
            </li>
          ))}
        </ol>
      )}
    </PopoverShell>
  );
}

export function PopoverShell({
  title,
  onClose,
  children,
}: {
  title: string;
  onClose: () => void;
  children: React.ReactNode;
}) {
  return (
    <div className="fixed inset-0 z-50 flex items-end justify-center bg-black/30 backdrop-blur-sm sm:items-center">
      <Card
        className="bg-popover relative m-3 max-h-[70vh] w-full max-w-md overflow-y-auto p-4 text-popover-foreground shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="mb-3 flex items-center justify-between">
          <h3 className="font-semibold capitalize">{title}</h3>
          <button
            type="button"
            onClick={onClose}
            className="text-muted-foreground hover:text-foreground"
            aria-label="Close"
          >
            <X className="size-4" />
          </button>
        </div>
        {children}
      </Card>
    </div>
  );
}
