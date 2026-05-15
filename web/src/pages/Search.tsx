import { useQuery } from "@tanstack/react-query";
import { Link, useLocation } from "react-router";
import { listLibraries, searchCatalog } from "@/lib/api";
import { BookCard } from "@/components/BookCard";
import { Skeleton } from "@/components/ui/skeleton";

export default function Search() {
  const loc = useLocation();
  const params = new URLSearchParams(loc.search);
  const q = params.get("q") ?? "";
  const libraryID = Number(params.get("library_id") || 0) || undefined;
  const libraries = useQuery({
    queryKey: ["libraries"],
    queryFn: listLibraries,
  });
  const r = useQuery({
    queryKey: ["search", q, libraryID],
    queryFn: () => searchCatalog(q, libraryID),
    enabled: !!q,
  });
  return (
    <div className="space-y-4">
      <h1 className="text-xl font-semibold">Search: {q}</h1>
      {libraries.data && libraries.data.items.length > 1 && (
        <nav className="flex flex-wrap gap-2">
          <FilterPill
            to={`/search?q=${encodeURIComponent(q)}`}
            active={!libraryID}
          >
            All
          </FilterPill>
          {libraries.data.items.map((library) => (
            <FilterPill
              key={library.id}
              to={`/search?q=${encodeURIComponent(q)}&library_id=${library.id}`}
              active={libraryID === library.id}
            >
              {library.name}
            </FilterPill>
          ))}
        </nav>
      )}
      {!q ? (
        <p className="text-sm text-muted-foreground">
          Type a query in the search bar.
        </p>
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

function FilterPill({
  to,
  active,
  children,
}: {
  to: string;
  active: boolean;
  children: React.ReactNode;
}) {
  return (
    <Link
      to={to}
      className={`rounded-full border px-3 py-1.5 text-xs font-medium ${
        active
          ? "border-primary/40 bg-primary/12 text-foreground"
          : "border-border bg-card text-muted-foreground hover:bg-accent"
      }`}
    >
      {children}
    </Link>
  );
}
