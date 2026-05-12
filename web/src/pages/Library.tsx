import { useQuery } from '@tanstack/react-query';
import { fetchLibrary } from '@/lib/api';
import { Skeleton } from '@/components/ui/skeleton';

export default function Library() {
  const lib = useQuery({ queryKey: ['library'], queryFn: () => fetchLibrary() });
  return (
    <div>
      <h1 className="mb-4 text-xl font-semibold">My library</h1>
      {lib.isLoading ? (
        <div className="space-y-2">
          {Array.from({ length: 6 }).map((_, i) => (
            <Skeleton key={i} className="h-12 w-full" />
          ))}
        </div>
      ) : lib.error ? (
        <p className="text-sm text-destructive">{(lib.error as Error).message}</p>
      ) : lib.data && lib.data.items.length > 0 ? (
        <ul className="space-y-2">
          {lib.data.items.map((row) => (
            <li
              key={row.book_id}
              className="flex items-center justify-between rounded-lg border border-border bg-card px-4 py-2"
            >
              <span className="font-mono text-xs">{row.book_id}</span>
              <span className="text-xs text-muted-foreground">
                {row.is_finished ? 'finished' : `${Math.round((row.read_progress ?? 0) * 100)}%`}
              </span>
            </li>
          ))}
        </ul>
      ) : (
        <p className="text-sm text-muted-foreground">
          Nothing in your library yet. Open a book from Home to start reading.
        </p>
      )}
    </div>
  );
}
