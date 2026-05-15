import { useQueries, useQuery } from "@tanstack/react-query";
import { Link } from "react-router";
import {
  getBook,
  listCatalog,
  listLibraries,
  listMyRequests,
  listRecentProgress,
  type EbookSummary,
} from "@/lib/api";
import { BookCard } from "@/components/BookCard";
import { Skeleton } from "@/components/ui/skeleton";

export default function Home() {
  const recent = useQuery({
    queryKey: ["catalog", "recent"],
    queryFn: () => listCatalog("", "added", "desc", 24),
  });
  const libraries = useQuery({
    queryKey: ["libraries"],
    queryFn: listLibraries,
  });
  const progress = useQuery({
    queryKey: ["recent-progress"],
    queryFn: listRecentProgress,
  });
  const requests = useQuery({
    queryKey: ["my-requests"],
    queryFn: listMyRequests,
  });
  const activeRequests = (requests.data?.items ?? []).filter((r) =>
    ["pending", "submitted", "acknowledged", "downloading"].includes(r.status),
  );
  const progressItems = (progress.data?.items ?? []).slice(0, 6);
  const progressBooks = useQueries({
    queries: progressItems.map((item) => ({
      queryKey: ["book", item.book_id],
      queryFn: () => getBook(item.book_id),
      enabled: !!item.book_id,
      retry: false,
    })),
  });

  return (
    <div className="space-y-8">
      <section className="grid gap-3 md:grid-cols-3">
        <DashboardLink
          title="Libraries"
          value={String(libraries.data?.items.length ?? 0)}
          to="/library"
        />
        <DashboardLink
          title="Continue reading"
          value={String(progress.data?.items.length ?? 0)}
          to="/library"
        />
        <DashboardLink
          title="Active requests"
          value={String(activeRequests.length)}
          to="/me/requests"
        />
      </section>

      {(progress.data?.items ?? []).length > 0 && (
        <section>
          <div className="mb-3 flex items-baseline justify-between">
            <h2 className="text-lg font-semibold tracking-tight">
              Continue reading
            </h2>
            <Link
              to="/library"
              className="text-xs text-muted-foreground hover:text-foreground"
            >
              View all →
            </Link>
          </div>
          <div className="grid gap-3 md:grid-cols-2 lg:grid-cols-3">
            {progressItems.map((item, index) => (
              <Link
                key={item.book_id}
                to={`/${encodeURIComponent(item.book_id)}/read`}
                className="rounded-lg border border-border bg-card p-4 hover:bg-accent"
              >
                <div className="truncate text-sm font-medium">
                  {progressBooks[index]?.data?.title || item.book_id}
                </div>
                <div className="truncate text-xs text-muted-foreground">
                  {(progressBooks[index]?.data?.authors ?? []).join(", ") ||
                    "Continue reading"}
                </div>
                <div className="mt-2 h-2 overflow-hidden rounded-full bg-muted">
                  <div
                    className="h-full bg-primary"
                    style={{
                      width: `${Math.min(100, Math.round((item.read_progress ?? 0) * 100))}%`,
                    }}
                  />
                </div>
                <div className="mt-1 text-xs text-muted-foreground">
                  {Math.round((item.read_progress ?? 0) * 100)}% read
                </div>
              </Link>
            ))}
          </div>
        </section>
      )}

      <section>
        <div className="mb-3 flex items-baseline justify-between">
          <h2 className="text-lg font-semibold tracking-tight">
            Recently added
          </h2>
          <Link
            to="/library"
            className="text-xs text-muted-foreground hover:text-foreground"
          >
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
          <p className="text-sm text-destructive">
            {(recent.error as Error).message}
          </p>
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

function DashboardLink({
  title,
  value,
  to,
}: {
  title: string;
  value: string;
  to: string;
}) {
  return (
    <Link
      to={to}
      className="rounded-lg border border-border bg-card p-4 hover:bg-accent"
    >
      <div className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
        {title}
      </div>
      <div className="mt-2 text-2xl font-semibold">{value}</div>
    </Link>
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
