import {
  useMutation,
  useQueries,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { useState } from "react";
import { Link } from "react-router";
import { toast } from "sonner";
import { Check, Pencil, Trash2, X } from "lucide-react";
import {
  createCollection,
  deleteCollection,
  listCollectionItems,
  listMyCollections,
  updateCollection,
  type Collection,
} from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";

export default function Collections() {
  const qc = useQueryClient();
  const q = useQuery({ queryKey: ["collections"], queryFn: listMyCollections });
  const itemQueries = useQueries({
    queries: (q.data?.items ?? []).map((collection) => ({
      queryKey: ["collections", collection.id, "items"],
      queryFn: () => listCollectionItems(collection.id),
    })),
  });
  const [name, setName] = useState("");
  const [isPublic, setIsPublic] = useState(false);
  const [isPinned, setIsPinned] = useState(false);
  const [editing, setEditing] = useState<Collection | null>(null);
  const create = useMutation({
    mutationFn: () =>
      createCollection({ name, is_public: isPublic, is_pinned: isPinned }),
    onSuccess: () => {
      setName("");
      setIsPublic(false);
      setIsPinned(false);
      qc.invalidateQueries({ queryKey: ["collections"] });
    },
    onError: (e: Error) => toast.error(e.message),
  });
  const del = useMutation({
    mutationFn: (id: string) => deleteCollection(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["collections"] }),
    onError: (e: Error) => toast.error(e.message),
  });
  const update = useMutation({
    mutationFn: (collection: Collection) =>
      updateCollection(collection.id, collection),
    onSuccess: () => {
      setEditing(null);
      qc.invalidateQueries({ queryKey: ["collections"] });
    },
    onError: (e: Error) => toast.error(e.message),
  });

  return (
    <div>
      <div className="mb-4">
        <h1 className="text-xl font-semibold">Collections</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Organize books into pinned, private, or shareable reading lists.
        </p>
      </div>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          if (name.trim()) create.mutate();
        }}
        className="mb-4 grid gap-2 rounded-lg border border-border bg-card p-4 md:grid-cols-[1fr_auto_auto_auto]"
      >
        <input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="New collection name"
          className="flex-1 rounded-md border border-border bg-background px-2 py-1.5 text-sm"
        />
        <label className="inline-flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={isPinned}
            onChange={(e) => setIsPinned(e.target.checked)}
          />
          Pinned
        </label>
        <label className="inline-flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={isPublic}
            onChange={(e) => setIsPublic(e.target.checked)}
          />
          Public
        </label>
        <Button type="submit" disabled={!name.trim() || create.isPending}>
          Create
        </Button>
      </form>
      {q.isLoading ? (
        <div className="space-y-2">
          {Array.from({ length: 3 }).map((_, i) => (
            <Skeleton key={i} className="h-12 w-full" />
          ))}
        </div>
      ) : q.data && q.data.items.length > 0 ? (
        <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
          {q.data.items.map((c, index) => (
            <li
              key={c.id}
              className="list-none rounded-lg border border-border bg-card p-4"
            >
              {editing?.id === c.id ? (
                <CollectionEditor
                  collection={editing}
                  itemCount={itemQueries[index]?.data?.items.length ?? 0}
                  onChange={setEditing}
                  onCancel={() => setEditing(null)}
                  onSave={() => update.mutate(editing)}
                  saving={update.isPending}
                />
              ) : (
                <div className="flex items-start justify-between gap-3">
                  <Link to={`/collections/${c.id}`} className="min-w-0">
                    <span className="block truncate text-sm font-medium hover:underline">
                      {c.name}
                    </span>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {itemQueries[index]?.data?.items.length ?? 0} items
                      {c.is_pinned ? " · pinned" : ""}
                      {c.is_public ? " · public" : " · private"}
                    </p>
                  </Link>
                  <div className="flex shrink-0 items-center gap-1">
                    <Button
                      size="icon"
                      variant="ghost"
                      onClick={() => setEditing(c)}
                    >
                      <Pencil className="size-4" />
                    </Button>
                    <Button
                      size="icon"
                      variant="ghost"
                      onClick={() => del.mutate(c.id)}
                      disabled={del.isPending}
                    >
                      <Trash2 className="size-4" />
                    </Button>
                  </div>
                </div>
              )}
            </li>
          ))}
        </div>
      ) : (
        <p className="text-sm text-muted-foreground">No collections yet.</p>
      )}
    </div>
  );
}

function CollectionEditor({
  collection,
  itemCount,
  onChange,
  onCancel,
  onSave,
  saving,
}: {
  collection: Collection;
  itemCount: number;
  onChange: (collection: Collection) => void;
  onCancel: () => void;
  onSave: () => void;
  saving: boolean;
}) {
  return (
    <div className="space-y-3">
      <input
        value={collection.name}
        onChange={(e) => onChange({ ...collection, name: e.target.value })}
        className="w-full rounded-md border border-border bg-background px-2 py-1.5 text-sm"
      />
      <p className="text-xs text-muted-foreground">{itemCount} items</p>
      <div className="flex flex-wrap items-center gap-3">
        <label className="inline-flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={!!collection.is_pinned}
            onChange={(e) =>
              onChange({ ...collection, is_pinned: e.target.checked })
            }
          />
          Pinned
        </label>
        <label className="inline-flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={!!collection.is_public}
            onChange={(e) =>
              onChange({ ...collection, is_public: e.target.checked })
            }
          />
          Public
        </label>
      </div>
      <div className="flex items-center gap-2">
        <Button
          size="sm"
          onClick={onSave}
          disabled={!collection.name.trim() || saving}
        >
          <Check className="mr-1 size-4" />
          Save
        </Button>
        <Button size="sm" variant="outline" onClick={onCancel}>
          <X className="mr-1 size-4" />
          Cancel
        </Button>
      </div>
    </div>
  );
}
