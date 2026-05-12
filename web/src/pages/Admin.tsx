import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useEffect, useState } from 'react';
import { toast } from 'sonner';
import {
  adminCacheStats,
  adminGetBackend,
  adminListRequests,
  adminPatchBackend,
  adminPatchRequest,
  fetchInstalledBackends,
} from '@/lib/api';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';

// Admin landing: combined view of (a) request queue, (b) backend config,
// (c) cache stats. Each section reloads independently.
export default function Admin() {
  return (
    <div className="space-y-8">
      <h1 className="text-xl font-semibold">Admin</h1>
      <RequestQueueSection />
      <BackendConfigSection />
      <CacheStatsSection />
    </div>
  );
}

function RequestQueueSection() {
  const qc = useQueryClient();
  const q = useQuery({
    queryKey: ['admin', 'requests'],
    queryFn: () => adminListRequests(),
  });
  const patch = useMutation({
    mutationFn: ({ id, action }: { id: string; action: string }) =>
      adminPatchRequest(id, { action }),
    onSuccess: () => {
      toast.success('Updated');
      qc.invalidateQueries({ queryKey: ['admin', 'requests'] });
    },
    onError: (e: Error) => toast.error(e.message),
  });
  return (
    <section>
      <h2 className="mb-2 text-base font-semibold">Request queue</h2>
      {q.isLoading ? (
        <Skeleton className="h-32 w-full" />
      ) : q.data && q.data.items.length > 0 ? (
        <ul className="space-y-2">
          {q.data.items.map((r) => (
            <li
              key={r.id}
              className="flex items-center justify-between gap-3 rounded-lg border border-border bg-card p-3"
            >
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm font-medium">{r.title}</p>
                <p className="truncate text-xs text-muted-foreground">
                  {r.status} · {r.target_plugin_id}
                </p>
              </div>
              {r.status === 'pending' && (
                <div className="flex gap-2">
                  <Button
                    size="sm"
                    onClick={() => patch.mutate({ id: r.id, action: 'approve' })}
                    disabled={patch.isPending}
                  >
                    Approve
                  </Button>
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() => patch.mutate({ id: r.id, action: 'deny' })}
                    disabled={patch.isPending}
                  >
                    Deny
                  </Button>
                </div>
              )}
            </li>
          ))}
        </ul>
      ) : (
        <p className="text-sm text-muted-foreground">No requests in queue.</p>
      )}
    </section>
  );
}

function BackendConfigSection() {
  const qc = useQueryClient();
  const cfg = useQuery({ queryKey: ['admin', 'backend'], queryFn: adminGetBackend });
  const backends = useQuery({ queryKey: ['installed-backends'], queryFn: fetchInstalledBackends });
  const [targetID, setTargetID] = useState('');
  const [autoApprove, setAutoApprove] = useState(false);
  const [mode, setMode] = useState('proxy');

  useEffect(() => {
    if (cfg.data) {
      setTargetID(cfg.data.target_backend_plugin_id);
      setAutoApprove(cfg.data.auto_approve_requests);
      setMode(cfg.data.default_streaming_mode || 'proxy');
    }
  }, [cfg.data]);

  const save = useMutation({
    mutationFn: () =>
      adminPatchBackend({
        target_backend_plugin_id: targetID,
        auto_approve_requests: autoApprove,
        default_streaming_mode: mode,
      }),
    onSuccess: () => {
      toast.success('Saved');
      qc.invalidateQueries({ queryKey: ['admin', 'backend'] });
    },
    onError: (e: Error) => toast.error(e.message),
  });

  if (cfg.isLoading) return <Skeleton className="h-32 w-full" />;
  return (
    <section>
      <h2 className="mb-2 text-base font-semibold">Backend config</h2>
      <div className="space-y-3 rounded-lg border border-border bg-card p-4">
        <div>
          <label className="mb-1 block text-xs font-medium text-muted-foreground">
            Target backend plugin
          </label>
          <select
            value={targetID}
            onChange={(e) => setTargetID(e.target.value)}
            className="w-full rounded-md border border-border bg-background px-2 py-1.5 text-sm"
          >
            <option value="">(none)</option>
            {backends.data?.map((b) => (
              <option key={b.id} value={String(b.id)}>
                {b.display_name} ({b.plugin_id})
              </option>
            ))}
          </select>
        </div>
        <label className="inline-flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={autoApprove}
            onChange={(e) => setAutoApprove(e.target.checked)}
          />
          Auto-approve new requests
        </label>
        <div>
          <label className="mb-1 block text-xs font-medium text-muted-foreground">
            Streaming mode
          </label>
          <select
            value={mode}
            onChange={(e) => setMode(e.target.value)}
            className="rounded-md border border-border bg-background px-2 py-1.5 text-sm"
          >
            <option value="proxy">Proxy (live forward)</option>
            <option value="cache">Cache (LRU disk)</option>
          </select>
        </div>
        <Button onClick={() => save.mutate()} disabled={save.isPending}>
          Save
        </Button>
      </div>
    </section>
  );
}

function CacheStatsSection() {
  const s = useQuery({ queryKey: ['admin', 'cache'], queryFn: adminCacheStats });
  if (s.isLoading) return <Skeleton className="h-20 w-full" />;
  if (!s.data) return null;
  const usedGB = s.data.bytes_used / 1024 / 1024 / 1024;
  const maxGB = s.data.bytes_max / 1024 / 1024 / 1024;
  const pct = maxGB ? (usedGB / maxGB) * 100 : 0;
  return (
    <section>
      <h2 className="mb-2 text-base font-semibold">Cache</h2>
      <div className="rounded-lg border border-border bg-card p-4">
        <p className="text-sm">
          {usedGB.toFixed(2)} GB / {maxGB.toFixed(2)} GB ({pct.toFixed(1)}%)
        </p>
        <div className="mt-2 h-2 w-full overflow-hidden rounded-full bg-muted">
          <div
            className="h-full bg-primary transition-all"
            style={{ width: `${Math.min(100, pct)}%` }}
          />
        </div>
      </div>
    </section>
  );
}
