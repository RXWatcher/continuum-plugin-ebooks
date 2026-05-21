import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Users } from "lucide-react";
import { Link, useSearchParams } from "react-router";
import { browseAuthors, type FacetItem, type PageEnvelope } from "@/lib/api";
import InfiniteFooter from "@/components/InfiniteFooter";
import { Skeleton } from "@/components/ui/skeleton";

export default function Authors() {
  const [params] = useSearchParams();
  const libraryID = Number(params.get("library_id") || 0) || undefined;
  const [pages, setPages] = useState<PageEnvelope<FacetItem>[]>([]);
  const [cursor, setCursor] = useState<string>("");
  const q = useQuery({
    queryKey: ["browse", "authors", cursor, libraryID],
    queryFn: async () => {
      const page = await browseAuthors(cursor, 50, libraryID);
      setPages((prev) => (cursor === "" ? [page] : [...prev, page]));
      return page;
    },
  });

  const items = pages.flatMap((p) => p.items);
  const last = pages[pages.length - 1];
  const nextCursor = last?.next_cursor ?? "";

  return (
    <div className="space-y-4">
      <header className="flex items-center gap-2">
        <Users className="size-5 text-muted-foreground" />
        <h1 className="text-xl font-semibold">Authors</h1>
        {last?.total ? (
          <span className="text-xs text-muted-foreground">
            ({last.total.toLocaleString()})
          </span>
        ) : null}
      </header>
      {q.isLoading && pages.length === 0 ? (
        <FacetSkeletonGrid />
      ) : q.error ? (
        <p className="text-sm text-destructive">{(q.error as Error).message}</p>
      ) : items.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          This backend doesn&rsquo;t support browsing by author &mdash; try
          search instead.
        </p>
      ) : (
        <>
          <FacetGrid items={items} libraryID={libraryID} />
          <InfiniteFooter
            hasNextPage={Boolean(nextCursor)}
            isFetchingNextPage={q.isFetching && pages.length > 0}
            fetchNextPage={() => setCursor(nextCursor)}
            label="authors"
          />
        </>
      )}
    </div>
  );
}

function FacetSkeletonGrid() {
  return (
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5">
      {Array.from({ length: 10 }).map((_, i) => (
        <Skeleton key={i} className="h-16 w-full rounded-lg" />
      ))}
    </div>
  );
}

function FacetGrid({
  items,
  libraryID,
}: {
  items: FacetItem[];
  libraryID?: number;
}) {
  return (
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5">
      {items.map((it) => (
        <Link
          key={it.id}
          to={`/library?${new URLSearchParams({
            ...(libraryID ? { library_id: String(libraryID) } : {}),
            author: it.name,
          }).toString()}`}
          className="block rounded-lg border border-border bg-card px-3 py-2.5 transition-colors hover:bg-surface-hover focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <h3 className="line-clamp-2 text-sm font-medium leading-snug text-foreground">
            {it.name}
          </h3>
          {typeof it.count === "number" && it.count > 0 && (
            <p className="mt-0.5 text-xs text-muted-foreground">
              {it.count.toLocaleString()} {it.count === 1 ? "book" : "books"}
            </p>
          )}
        </Link>
      ))}
    </div>
  );
}
