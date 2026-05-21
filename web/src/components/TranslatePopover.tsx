import { useEffect, useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { PopoverShell } from "./DefinePopover";

// TranslatePopover sends the selected text to the LibreTranslate-
// compatible /translate endpoint. Target language defaults to the
// browser locale; user can change before re-running.

const TARGETS: { value: string; label: string }[] = [
  { value: "en", label: "English" },
  { value: "es", label: "Spanish" },
  { value: "fr", label: "French" },
  { value: "de", label: "German" },
  { value: "it", label: "Italian" },
  { value: "pt", label: "Portuguese" },
  { value: "nl", label: "Dutch" },
  { value: "ja", label: "Japanese" },
  { value: "zh", label: "Chinese" },
  { value: "ko", label: "Korean" },
];

export default function TranslatePopover({
  text,
  onClose,
}: {
  text: string;
  onClose: () => void;
}) {
  const browserLang = (navigator.language ?? "en").split("-")[0];
  const [target, setTarget] = useState(
    TARGETS.some((t) => t.value === browserLang) ? browserLang : "en",
  );

  const realTranslate = useMutation({
    mutationFn: () =>
      api.post<{ translated_text: string; source_lang?: string }>(`/translate`, {
        text,
        source: "auto",
        target,
      }),
  });

  // Kick off the first translation on mount + whenever the target
  // changes. realTranslate.data carries the latest result.
  useEffect(() => {
    realTranslate.mutate();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [target, text]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  return (
    <PopoverShell title="Translate" onClose={onClose}>
      <div className="mb-3">
        <div className="text-muted-foreground mb-1 text-xs uppercase tracking-wide">
          Original
        </div>
        <p className="text-sm italic">"{truncate(text, 280)}"</p>
      </div>

      <div className="mb-3 flex items-center gap-2">
        <span className="text-muted-foreground text-xs">Translate to:</span>
        <Select value={target} onValueChange={setTarget}>
          <SelectTrigger className="h-8 w-32">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {TARGETS.map((t) => (
              <SelectItem key={t.value} value={t.value}>
                {t.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Button
          size="sm"
          variant="ghost"
          onClick={() => realTranslate.mutate()}
          disabled={realTranslate.isPending}
        >
          Refresh
        </Button>
      </div>

      <div>
        <div className="text-muted-foreground mb-1 text-xs uppercase tracking-wide">
          Translation
        </div>
        {realTranslate.isPending ? (
          <Skeleton className="h-16 w-full" />
        ) : realTranslate.isError ? (
          <p className="text-destructive text-sm">
            {realTranslate.error instanceof Error
              ? realTranslate.error.message
              : "Translation failed"}
          </p>
        ) : (
          <p className="text-sm">{realTranslate.data?.translated_text ?? ""}</p>
        )}
      </div>
    </PopoverShell>
  );
}

function truncate(s: string, n: number): string {
  if (s.length <= n) return s;
  return s.slice(0, n - 1) + "…";
}

