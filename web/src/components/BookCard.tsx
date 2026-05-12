import { Link } from 'react-router';
import { BookOpen } from 'lucide-react';
import { mountPath, type EbookSummary } from '@/lib/api';

export function BookCard({ book }: { book: EbookSummary }) {
  // Cover URL: backends return either an absolute URL or a relative path on
  // their /cover/ endpoint. We rewrite relative paths through the plugin
  // proxy so the browser can fetch them.
  const cover = book.cover_url
    ? book.cover_url.startsWith('http')
      ? book.cover_url
      : `${mountPath()}${book.cover_url}`
    : '';
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
          <p className="line-clamp-1 text-xs text-muted-foreground">{book.authors.join(', ')}</p>
        )}
        {book.series && (
          <p className="line-clamp-1 text-[10px] text-muted-foreground/70">
            {book.series}
            {book.series_index ? ` #${book.series_index}` : ''}
          </p>
        )}
      </div>
    </Link>
  );
}
