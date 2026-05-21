import { Link } from "react-router";
import { BookOpen } from "lucide-react";
import { mountPath, type EbookSummary } from "@/lib/api";

export function BookCard({ book }: { book: EbookSummary }) {
  // Cover URL: backends now return a portal-signed absolute path that
  // routes directly through the host plugin proxy
  // (`/api/v1/plugins/{backend_id}/api/v1/cover/...?token=...`). Older
  // backends emit a path relative to *their* root (no `/api/v1/plugins/`
  // prefix), which the portal still needs to wrap via mountPath. Anything
  // already absolute (http: or `/api/v1/plugins/`) goes through unchanged
  // — double-wrapping turns `/api/v1/plugins/39/...` into
  // `/api/v1/plugins/44/api/v1/plugins/39/...` and the host router 404s.
  const cover = book.cover_url
    ? /^(https?:|\/api\/v1\/plugins\/)/.test(book.cover_url)
      ? book.cover_url
      : `${mountPath()}${book.cover_url}`
    : "";
  return (
    <Link
      to={`/${encodeURIComponent(book.id)}`}
      className="group block overflow-hidden rounded-lg border border-border bg-card transition-colors hover:bg-surface-hover"
    >
      <div className="relative aspect-[2/3] w-full bg-muted">
        {cover ? (
          <img
            src={cover}
            alt={book.title}
            loading="lazy"
            className="absolute inset-0 h-full w-full object-cover"
          />
        ) : (
          <div className="flex h-full w-full items-center justify-center text-muted-foreground">
            <BookOpen className="size-10" />
          </div>
        )}
      </div>
      <div className="space-y-1 p-2">
        <h3 className="line-clamp-2 text-xs font-semibold leading-tight group-hover:text-primary">
          {book.title}
        </h3>
        {book.authors && book.authors.length > 0 && (
          <p className="line-clamp-1 text-xs text-muted-foreground">
            {book.authors.join(", ")}
          </p>
        )}
        {book.series && (
          <p className="line-clamp-1 text-[10px] text-muted-foreground/70">
            {book.series}
            {book.series_index ? ` #${book.series_index}` : ""}
          </p>
        )}
      </div>
    </Link>
  );
}
