import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { toast } from 'sonner';
import { Trash2 } from 'lucide-react';
import { createCollection, deleteCollection, listMyCollections } from '@/lib/api';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';

export default function Collections() {
  const qc = useQueryClient();
  const q = useQuery({ queryKey: ['collections'], queryFn: listMyCollections });
  const [name, setName] = useState('');
  const create = useMutation({
    mutationFn: () => createCollection({ name }),
    onSuccess: () => {
      setName('');
      qc.invalidateQueries({ queryKey: ['collections'] });
    },
    onError: (e: Error) => toast.error(e.message),
  });
  const del = useMutation({
    mutationFn: (id: string) => deleteCollection(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['collections'] }),
    onError: (e: Error) => toast.error(e.message),
  });

  return (
    <div>
      <h1 className="mb-4 text-xl font-semibold">Collections</h1>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          if (name.trim()) create.mutate();
        }}
        className="mb-4 flex gap-2"
      >
        <input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="New collection name"
          className="flex-1 rounded-md border border-border bg-background px-2 py-1.5 text-sm"
        />
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
        <ul className="space-y-2">
          {q.data.items.map((c) => (
            <li
              key={c.id}
              className="flex items-center justify-between rounded-lg border border-border bg-card px-4 py-2"
            >
              <span className="text-sm font-medium">{c.name}</span>
              <Button
                size="icon"
                variant="ghost"
                onClick={() => del.mutate(c.id)}
                disabled={del.isPending}
              >
                <Trash2 className="size-4" />
              </Button>
            </li>
          ))}
        </ul>
      ) : (
        <p className="text-sm text-muted-foreground">No collections yet.</p>
      )}
    </div>
  );
}
