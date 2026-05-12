import { useQuery } from '@tanstack/react-query';
import { useLocation } from 'react-router';
import { searchCatalog } from '@/lib/api';
import { BookCard } from '@/components/BookCard';
import { Skeleton } from '@/components/ui/skeleton';

export default function Search() {
  const loc = useLocation();
  const q = new URLSearchParams(loc.search).get('q') ?? '';
  const r = useQuery({
    queryKey: ['search', q],
    queryFn: () => searchCatalog(q),
    enabled: !!q,
  });
  return (
    <div className="space-y-4">
      <h1 className="text-xl font-semibold">Search: {q}</h1>
      {!q ? (
        <p className="text-sm text-muted-foreground">Type a query in the search bar.</p>
      ) : r.isLoading ? (
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6">
          {Array.from({ length: 12 }).map((_, i) => (
            <Skeleton key={i} className="aspect-[2/3] w-full rounded-lg" />
          ))}
        </div>
      ) : r.error ? (
        <p className="text-sm text-destructive">{(r.error as Error).message}</p>
      ) : r.data && r.data.items.length > 0 ? (
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6">
          {r.data.items.map((b) => (
            <BookCard key={b.id} book={b} />
          ))}
        </div>
      ) : (
        <p className="text-sm text-muted-foreground">No results.</p>
      )}
    </div>
  );
}
