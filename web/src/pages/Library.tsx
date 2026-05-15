import { useQuery } from "@tanstack/react-query";
import { Link, useSearchParams } from "react-router";
import { X } from "lucide-react";
import { listCatalog, listLibraries, type CatalogFilters } from "@/lib/api";
import { BookCard } from "@/components/BookCard";
import { Skeleton } from "@/components/ui/skeleton";

// Library is the catalog browser. Optional filter query params drive a server-
// side filter on the backend's /catalog endpoint (which itself proxies to
// provider. When any filter is active a chip is shown with a clear (×)
// link back to /library.
export default function Library() {
  const [params] = useSearchParams();
  const libraryID = Number(params.get("library_id") || 0) || undefined;
  const filters: CatalogFilters = {
    library_id: libraryID,
    author: params.get("author") || undefined,
    series: params.get("series") || undefined,
    genre: params.get("genre") || undefined,
    tag: params.get("tag") || undefined,
  };
  const activeFilter = pickActiveFilter(filters);

  const libraries = useQuery({
    queryKey: ["catalog", "libraries"],
    queryFn: listLibraries,
  });
  const q = useQuery({
    queryKey: ["catalog", "library", filters],
    queryFn: () => listCatalog("", "added", "desc", 50, filters),
  });

  return (
    <div className="space-y-4">
      <header className="flex items-center justify-between gap-2">
        <h1 className="text-xl font-semibold">Library</h1>
        {q.data?.total ? (
          <span className="text-xs text-muted-foreground">
            {q.data.total.toLocaleString()}{" "}
            {q.data.total === 1 ? "book" : "books"}
          </span>
        ) : null}
      </header>

      {libraries.data && libraries.data.items.length > 1 && (
        <nav className="flex flex-wrap gap-2" aria-label="Libraries">
          <LibraryPill to="/library" active={!libraryID}>
            All
          </LibraryPill>
          {libraries.data.items.map((library) => (
            <LibraryPill
              key={library.id}
              to={`/library?library_id=${library.id}`}
              active={library.id === libraryID}
            >
              {library.name}
              {library.media_type && library.media_type !== "book" ? (
                <span className="text-[10px] font-normal uppercase text-muted-foreground">
                  {library.media_type}
                </span>
              ) : null}
            </LibraryPill>
          ))}
        </nav>
      )}

      {activeFilter && (
        <div className="flex items-center gap-2">
          <span className="inline-flex items-center gap-2 rounded-full border border-border bg-card px-3 py-1 text-xs text-foreground">
            <span className="text-muted-foreground">{activeFilter.label}:</span>
            <span className="font-medium">{activeFilter.value}</span>
            <Link
              to={libraryID ? `/library?library_id=${libraryID}` : "/library"}
              aria-label="Clear filter"
              className="-mr-1 rounded-full p-0.5 text-muted-foreground transition-colors hover:bg-surface-hover hover:text-foreground"
            >
              <X className="size-3.5" />
            </Link>
          </span>
        </div>
      )}

      {q.isLoading ? (
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6">
          {Array.from({ length: 12 }).map((_, i) => (
            <Skeleton key={i} className="aspect-[2/3] w-full rounded-lg" />
          ))}
        </div>
      ) : q.error ? (
        <p className="text-sm text-destructive">{(q.error as Error).message}</p>
      ) : q.data && q.data.items.length > 0 ? (
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6">
          {q.data.items.map((b) => (
            <BookCard key={b.id} book={b} />
          ))}
        </div>
      ) : (
        <p className="text-sm text-muted-foreground">
          {activeFilter ? "No books match this filter." : "No ebooks yet."}
        </p>
      )}
    </div>
  );
}

function LibraryPill({
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
      className={`inline-flex items-center gap-1.5 rounded-full border px-3 py-1.5 text-xs font-medium transition-colors ${
        active
          ? "border-primary/40 bg-primary/12 text-foreground"
          : "border-border bg-card text-muted-foreground hover:bg-surface-hover hover:text-foreground"
      }`}
    >
      {children}
    </Link>
  );
}

function pickActiveFilter(
  f: CatalogFilters,
): { label: string; value: string } | null {
  if (f.author) return { label: "Author", value: f.author };
  if (f.series) return { label: "Series", value: f.series };
  if (f.genre) return { label: "Genre", value: f.genre };
  if (f.tag) return { label: "Tag", value: f.tag };
  return null;
}
