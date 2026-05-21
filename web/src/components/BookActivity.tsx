import { useQuery } from "@tanstack/react-query";
import {
  BookOpen,
  Highlighter,
  History,
  Share2,
  Star,
  Trophy,
} from "lucide-react";
import { getBookActivity, type ActivityEvent } from "@/lib/api";
import { Card } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";

// Per-book activity timeline for the ebook detail page. Mirrors
// the audiobook component's layout with ebook-specific kinds
// (progress / finished / rated / annotation / shared).

const KIND_META: Record<
  string,
  {
    icon: React.ComponentType<{ className?: string }>;
    label: (p: any) => string;
  }
> = {
  progress: {
    icon: BookOpen,
    label: (p) =>
      `Read ${Math.round((Number(p?.read_progress) || 0) * 100)}%${
        p?.current_page ? ` (page ${p.current_page})` : ""
      }`,
  },
  finished: {
    icon: Trophy,
    label: () => "Finished",
  },
  rated: {
    icon: Star,
    label: (p) => `Rated ${p?.rating ?? 0}/5`,
  },
  annotation: {
    icon: Highlighter,
    label: (p) => {
      if (p?.note_text) return `Note: ${truncate(String(p.note_text), 80)}`;
      if (p?.selected_text)
        return `${capitalize(String(p.style ?? "highlight"))}: ${truncate(
          String(p.selected_text),
          80,
        )}`;
      return capitalize(String(p?.style ?? "annotation"));
    },
  },
  shared: {
    icon: Share2,
    label: () => "Created a share link",
  },
};

export default function BookActivity({ bookId }: { bookId: string }) {
  const activity = useQuery({
    queryKey: ["book-activity", bookId],
    queryFn: () => getBookActivity(bookId),
    enabled: !!bookId,
  });

  return (
    <Card className="bg-surface p-4">
      <div className="mb-3 flex items-center gap-2">
        <History className="size-5" />
        <h3 className="font-medium">Activity</h3>
      </div>
      {activity.isLoading ? (
        <Skeleton className="h-24 w-full" />
      ) : (activity.data?.events ?? []).length === 0 ? (
        <p className="text-muted-foreground text-sm">
          No activity yet — reading progress, annotations, and shares appear
          here.
        </p>
      ) : (
        <ol className="space-y-3">
          {activity.data!.events.slice(0, 50).map((ev, i) => (
            <EventRow key={`${ev.at}-${i}`} event={ev} />
          ))}
        </ol>
      )}
    </Card>
  );
}

function EventRow({ event }: { event: ActivityEvent }) {
  const meta = KIND_META[event.kind];
  const Icon = meta?.icon ?? History;
  const label = meta ? meta.label(event.payload) : event.kind;
  return (
    <li className="flex items-start gap-3">
      <div className="bg-background mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-full">
        <Icon className="size-4" />
      </div>
      <div className="min-w-0 flex-1">
        <div className="text-sm">{label}</div>
        <div className="text-muted-foreground text-xs tabular-nums">
          {formatWhen(event.at)}
        </div>
      </div>
    </li>
  );
}

function formatWhen(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const diffMs = Date.now() - d.getTime();
  const minute = 60_000;
  const hour = minute * 60;
  const day = hour * 24;
  if (diffMs < minute) return "just now";
  if (diffMs < hour) return `${Math.floor(diffMs / minute)}m ago`;
  if (diffMs < day) return `${Math.floor(diffMs / hour)}h ago`;
  if (diffMs < day * 30) return `${Math.floor(diffMs / day)}d ago`;
  return d.toLocaleDateString();
}

function truncate(s: string, n: number): string {
  if (s.length <= n) return s;
  return s.slice(0, n - 1) + "…";
}

function capitalize(s: string): string {
  if (!s) return s;
  return s.charAt(0).toUpperCase() + s.slice(1);
}
