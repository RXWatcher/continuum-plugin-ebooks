import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useParams, Link } from 'react-router';
import { toast } from 'sonner';
import {
  getBook,
  sendToKindle,
  sendToKobo,
  mountPath,
  type EbookDetail,
} from '@/lib/api';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { BookOpen, Send, Smartphone, Download } from 'lucide-react';
import { useState } from 'react';

export default function BookDetail() {
  const params = useParams();
  const id = params.id ?? '';
  const qc = useQueryClient();
  const book = useQuery<EbookDetail>({
    queryKey: ['book', id],
    queryFn: () => getBook(id),
    enabled: !!id,
  });

  const [kindleAddr, setKindleAddr] = useState('');
  const kindleM = useMutation({
    mutationFn: (format: string) => sendToKindle(id, format, kindleAddr),
    onSuccess: () => toast.success('Queued for Kindle'),
    onError: (e: Error) => toast.error(e.message),
  });
  const koboM = useMutation({
    mutationFn: () => sendToKobo(id),
    onSuccess: (data) => {
      toast.success(`Kobo code: ${data.transfer_code}`);
      qc.invalidateQueries({ queryKey: ['book', id] });
    },
    onError: (e: Error) => toast.error(e.message),
  });

  if (book.isLoading) {
    return (
      <div className="grid grid-cols-1 gap-8 md:grid-cols-[200px_1fr]">
        <Skeleton className="aspect-[2/3] w-full rounded-lg" />
        <div className="space-y-4">
          <Skeleton className="h-8 w-3/4" />
          <Skeleton className="h-4 w-1/3" />
          <Skeleton className="h-24 w-full" />
        </div>
      </div>
    );
  }
  if (book.error) {
    return <p className="text-sm text-destructive">{(book.error as Error).message}</p>;
  }
  if (!book.data) return null;
  const b = book.data;
  const cover = b.cover_url
    ? b.cover_url.startsWith('http')
      ? b.cover_url
      : `${mountPath()}${b.cover_url}`
    : '';

  return (
    <div className="grid grid-cols-1 gap-8 md:grid-cols-[220px_1fr]">
      <div>
        <div className="aspect-[2/3] w-full overflow-hidden rounded-lg border border-border bg-muted">
          {cover ? (
            <img src={cover} alt={b.title} className="h-full w-full object-cover" />
          ) : (
            <div className="flex h-full items-center justify-center text-muted-foreground">
              <BookOpen className="size-10" />
            </div>
          )}
        </div>
      </div>
      <div className="space-y-4">
        <header>
          <h1 className="text-2xl font-bold tracking-tight">{b.title}</h1>
          {b.authors && (
            <p className="text-muted-foreground">{b.authors.join(', ')}</p>
          )}
          {b.series && (
            <p className="text-sm text-muted-foreground">
              {b.series}
              {b.series_index ? ` #${b.series_index}` : ''}
            </p>
          )}
        </header>
        {b.description && (
          <p className="text-sm leading-relaxed text-muted-foreground">{b.description}</p>
        )}
        <div className="flex flex-wrap items-center gap-2">
          <Button asChild>
            <Link to={`/${encodeURIComponent(id)}/read`}>
              <BookOpen className="mr-2 size-4" /> Read
            </Link>
          </Button>
          {b.files.map((f) => (
            <Button asChild key={f.format} variant="outline" size="sm">
              <a
                href={`${mountPath()}/api/v1/me/books/${encodeURIComponent(id)}/file?format=${f.format}`}
              >
                <Download className="mr-2 size-4" /> {f.format.toUpperCase()}
              </a>
            </Button>
          ))}
        </div>
        <div className="grid gap-3 md:grid-cols-2">
          <div className="rounded-lg border border-border bg-card p-3">
            <p className="mb-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
              <Send className="mr-1 inline size-3" /> Send to Kindle
            </p>
            <div className="flex items-center gap-2">
              <input
                value={kindleAddr}
                onChange={(e) => setKindleAddr(e.target.value)}
                placeholder="you@kindle.com"
                className="flex-1 rounded-md border border-border bg-background px-2 py-1.5 text-xs"
              />
              <Button
                size="sm"
                disabled={!kindleAddr.includes('@') || kindleM.isPending}
                onClick={() => kindleM.mutate('epub')}
              >
                Send
              </Button>
            </div>
          </div>
          <div className="rounded-lg border border-border bg-card p-3">
            <p className="mb-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
              <Smartphone className="mr-1 inline size-3" /> Send to Kobo
            </p>
            <Button
              size="sm"
              variant="outline"
              disabled={koboM.isPending}
              onClick={() => koboM.mutate()}
            >
              {koboM.isPending ? 'Preparing…' : 'Generate transfer code'}
            </Button>
          </div>
        </div>
        {b.genres && b.genres.length > 0 && (
          <div className="flex flex-wrap gap-1.5 pt-3">
            {b.genres.map((g) => (
              <span
                key={g}
                className="rounded-full border border-border bg-surface px-2 py-0.5 text-xs text-muted-foreground"
              >
                {g}
              </span>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
