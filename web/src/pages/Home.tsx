import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router';
import { listCatalog, type EbookSummary } from '@/lib/api';
import { BookCard } from '@/components/BookCard';
import { Skeleton } from '@/components/ui/skeleton';

export default function Home() {
  const recent = useQuery({
    queryKey: ['catalog', 'recent'],
    queryFn: () => listCatalog('', 'added', 'desc', 24),
  });

  return (
    <div className="space-y-8">
      <section>
        <div className="mb-3 flex items-baseline justify-between">
          <h2 className="text-lg font-semibold tracking-tight">Recently added</h2>
          <Link to="/library" className="text-xs text-muted-foreground hover:text-foreground">
            View library →
          </Link>
        </div>
        {recent.isLoading ? (
          <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6">
            {Array.from({ length: 12 }).map((_, i) => (
              <Skeleton key={i} className="aspect-[2/3] w-full rounded-lg" />
            ))}
          </div>
        ) : recent.error ? (
          <p className="text-sm text-destructive">{(recent.error as Error).message}</p>
        ) : recent.data && recent.data.items.length > 0 ? (
          <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6">
            {recent.data.items.map((b: EbookSummary) => (
              <BookCard key={b.id} book={b} />
            ))}
          </div>
        ) : (
          <EmptyState />
        )}
      </section>
    </div>
  );
}

function EmptyState() {
  return (
    <div className="rounded-lg border border-dashed border-border p-12 text-center">
      <p className="text-sm text-muted-foreground">
        No ebooks yet — connect a backend in the admin panel to get started.
      </p>
    </div>
  );
}
