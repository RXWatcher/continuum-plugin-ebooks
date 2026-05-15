import {
  useMutation,
  useQueries,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { Link, useParams } from "react-router";
import { toast } from "sonner";
import { BookOpen, ChevronLeft, Trash2 } from "lucide-react";
import {
  addCollectionItem,
  getBook,
  listCollectionItems,
  listMyCollections,
  removeCollectionItem,
  searchCatalog,
} from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { useState } from "react";

export default function CollectionDetail() {
  const { id = "" } = useParams();
  const qc = useQueryClient();
  const [query, setQuery] = useState("");
  const collections = useQuery({
    queryKey: ["collections"],
    queryFn: listMyCollections,
  });
  const items = useQuery({
    queryKey: ["collections", id, "items"],
    queryFn: () => listCollectionItems(id),
    enabled: !!id,
  });
  const books = useQueries({
    queries: (items.data?.items ?? []).map((item) => ({
      queryKey: ["book", item.book_id],
      queryFn: () => getBook(item.book_id),
      retry: false,
    })),
  });
  const search = useQuery({
    queryKey: ["collection-add-search", query],
    queryFn: () => searchCatalog(query),
    enabled: query.trim().length >= 3,
  });
  const add = useMutation({
    mutationFn: (bookID: string) => addCollectionItem(id, bookID),
    onSuccess: () => {
      toast.success("Added to collection");
      qc.invalidateQueries({ queryKey: ["collections", id, "items"] });
    },
    onError: (e: Error) => toast.error(e.message),
  });
  const remove = useMutation({
    mutationFn: (bookID: string) => removeCollectionItem(id, bookID),
    onSuccess: () => {
      toast.success("Removed from collection");
      qc.invalidateQueries({ queryKey: ["collections", id, "items"] });
    },
    onError: (e: Error) => toast.error(e.message),
  });
  const collection = collections.data?.items.find((item) => item.id === id);

  return (
    <div className="space-y-4">
      <Button asChild variant="ghost" size="sm">
        <Link to="/collections">
          <ChevronLeft className="mr-1 size-4" />
          Collections
        </Link>
      </Button>
      <header>
        <h1 className="text-xl font-semibold">
          {collection?.name || "Collection"}
        </h1>
        <p className="mt-1 text-sm text-muted-foreground">
          {items.data?.items.length ?? 0} saved titles
          {collection?.is_public ? " · public" : " · private"}
          {collection?.is_pinned ? " · pinned" : ""}
        </p>
      </header>
      <div className="rounded-lg border border-border bg-card p-3">
        <label className="mb-1 block text-xs font-medium text-muted-foreground">
          Add from library
        </label>
        <input
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Search by title, author, ISBN"
          className="w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
        />
        {(search.data?.items ?? []).length > 0 && (
          <div className="mt-2 grid gap-2 md:grid-cols-2">
            {(search.data?.items ?? []).slice(0, 6).map((book) => (
              <button
                key={book.id}
                type="button"
                disabled={add.isPending}
                onClick={() => add.mutate(book.id)}
                className="rounded-md border border-border bg-background px-3 py-2 text-left text-sm hover:bg-accent"
              >
                <span className="block truncate font-medium">{book.title}</span>
                <span className="block truncate text-xs text-muted-foreground">
                  {(book.authors ?? []).join(", ") || "Unknown author"}
                </span>
              </button>
            ))}
          </div>
        )}
      </div>
      {items.isLoading ? (
        <div className="space-y-2">
          {Array.from({ length: 4 }).map((_, index) => (
            <Skeleton key={index} className="h-16 w-full" />
          ))}
        </div>
      ) : (items.data?.items ?? []).length === 0 ? (
        <div className="rounded-lg border border-dashed border-border p-8 text-center text-sm text-muted-foreground">
          Add books from a book detail page to build this collection.
        </div>
      ) : (
        <div className="space-y-2">
          {(items.data?.items ?? []).map((item, index) => {
            const book = books[index]?.data;
            return (
              <div
                key={item.book_id}
                className="flex items-center justify-between gap-3 rounded-lg border border-border bg-card p-3"
              >
                <Link
                  to={`/${encodeURIComponent(item.book_id)}`}
                  className="min-w-0 flex-1"
                >
                  <div className="truncate text-sm font-medium">
                    {book?.title || item.book_id}
                  </div>
                  <div className="truncate text-xs text-muted-foreground">
                    {(book?.authors ?? []).join(", ") || "Collection item"}
                  </div>
                </Link>
                <Button asChild variant="outline" size="sm">
                  <Link to={`/${encodeURIComponent(item.book_id)}/read`}>
                    <BookOpen className="mr-1 size-4" />
                    Read
                  </Link>
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  disabled={remove.isPending}
                  onClick={() => remove.mutate(item.book_id)}
                >
                  <Trash2 className="size-4" />
                </Button>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
