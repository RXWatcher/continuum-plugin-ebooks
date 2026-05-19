import {
  useMutation,
  useQueries,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import {
  AlertCircle,
  CheckCircle2,
  Database,
  Gauge,
  HardDrive,
  KeyRound,
  LibraryBig,
  Mail,
  RefreshCw,
  Save,
  Send,
  Smartphone,
  Trash2,
  XCircle,
} from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import {
  adminCacheLargest,
  adminCacheStats,
  adminBulkRequests,
  adminDeleteKosyncUser,
  adminGetBackend,
  adminKindleLog,
  adminKoboSessions,
  adminKosyncUsers,
  adminListRoutingRules,
  adminListBackendLibraries,
  adminListLibraries,
  adminListRequests,
  adminOPDSTokens,
  adminPatchBackend,
  adminPatchRequest,
  adminProviderHealth,
  adminProviderTestSearch,
  adminReplaceLibraries,
  adminReplaceRoutingRules,
  adminSyncLibraries,
  adminRevokeOPDSToken,
  fetchDownloadProviders,
  fetchInstalledBackends,
  type BackendConfig,
  type InstalledBackend,
  type LibraryInfo,
  type Request,
  type RequestRoutingRule,
} from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

type BackendOption = InstalledBackend;
type Row = Record<string, unknown>;

const requestStatuses = [
  "",
  "pending",
  "submitted",
  "acknowledged",
  "downloading",
  "fulfilled",
  "denied",
  "failed",
];
const trackedRequestStatuses = requestStatuses.filter(Boolean);

function providerProfile(provider: InstalledBackend) {
  const supportsAutoMonitoring =
    provider.ebook_backend?.metadata?.supports_auto_monitoring === true;
  return {
    role: "Download provider",
    summary:
      provider.summary ||
      "Accepts ebook request events and returns status updates.",
    bestFor:
      "Provider-specific acquisition flows exposed through the ebook request router.",
    requirements: supportsAutoMonitoring
      ? "The plugin must be enabled and configured. This provider advertises auto-monitoring support."
      : "The plugin must be enabled and configured. Request handling depends on provider capabilities.",
  };
}

function providerLabel(providers: BackendOption[], installID?: string) {
  if (!installID) return "Default provider";
  const provider = providers.find((item) => String(item.id) === installID);
  return provider ? provider.display_name : `Install ${installID}`;
}

export default function Admin() {
  const qc = useQueryClient();
  const [tab, setTab] = useState("libraries");
  const [providerFilter, setProviderFilter] = useState("");
  const backend = useQuery({
    queryKey: ["admin", "backend"],
    queryFn: adminGetBackend,
  });
  const libraries = useQuery({
    queryKey: ["admin", "libraries"],
    queryFn: adminListLibraries,
  });
  const backends = useQuery({
    queryKey: ["installed-backends"],
    queryFn: fetchInstalledBackends,
  });
  const providers = useQuery({
    queryKey: ["download-providers"],
    queryFn: fetchDownloadProviders,
  });
  const cache = useQuery({
    queryKey: ["admin", "cache"],
    queryFn: adminCacheStats,
  });
  const requests = useQuery({
    queryKey: ["admin", "requests", ""],
    queryFn: () => adminListRequests(""),
  });

  const refreshAll = () => {
    qc.invalidateQueries({ queryKey: ["admin"] });
    qc.invalidateQueries({ queryKey: ["installed-backends"] });
    qc.invalidateQueries({ queryKey: ["download-providers"] });
  };

  const activeLibraries = (libraries.data?.items ?? []).filter(
    (l) => l.enabled,
  ).length;
  const pendingRequests = (requests.data?.items ?? []).filter(
    (r) => r.status === "pending",
  ).length;
  const cachePercent = cache.data?.bytes_max
    ? (cache.data.bytes_used / cache.data.bytes_max) * 100
    : 0;

  return (
    <div className="space-y-5">
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            Ebooks Administration
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Manage presentation libraries, request flow, cache behavior, reader
            integrations, and delivery queues.
          </p>
        </div>
        <Button type="button" variant="outline" onClick={refreshAll}>
          <RefreshCw className="size-4" />
          Refresh
        </Button>
      </header>

      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
        <MetricCard
          icon={<LibraryBig className="size-4" />}
          label="Enabled libraries"
          value={String(activeLibraries)}
          detail={`${(libraries.data?.items ?? []).length} presentation routes configured`}
        />
        <MetricCard
          icon={<Database className="size-4" />}
          label="Library sources"
          value={String(backends.data?.length ?? 0)}
          detail={`${providers.data?.length ?? 0} request/download providers`}
        />
        <MetricCard
          icon={<Send className="size-4" />}
          label="Pending requests"
          value={String(pendingRequests)}
          detail={`${(requests.data?.items ?? []).length} active queue items`}
        />
        <MetricCard
          icon={<HardDrive className="size-4" />}
          label="Cache used"
          value={formatBytes(cache.data?.bytes_used ?? 0)}
          detail={`${cachePercent.toFixed(1)}% of ${formatBytes(cache.data?.bytes_max ?? 0)}`}
        />
      </div>

      <Tabs value={tab} onValueChange={setTab} className="space-y-4">
        <TabsList className="flex h-auto w-full flex-wrap justify-start">
          <TabsTrigger value="libraries">Libraries</TabsTrigger>
          <TabsTrigger value="requests">Requests</TabsTrigger>
          <TabsTrigger value="providers">Providers</TabsTrigger>
          <TabsTrigger value="cache">Cache</TabsTrigger>
          <TabsTrigger value="integrations">Reader integrations</TabsTrigger>
          <TabsTrigger value="delivery">Delivery</TabsTrigger>
          <TabsTrigger value="settings">Settings</TabsTrigger>
        </TabsList>

        <TabsContent value="libraries">
          <LibrariesTab
            backend={backend.data}
            libraries={libraries.data?.items ?? []}
            backends={backends.data ?? []}
            backendsError={
              backends.error instanceof Error ? backends.error.message : null
            }
            loading={
              backend.isLoading || libraries.isLoading || backends.isLoading
            }
          />
        </TabsContent>
        <TabsContent value="requests">
          <RequestsTab
            providers={providers.data ?? []}
            providerFilter={providerFilter}
            onProviderFilterChange={setProviderFilter}
          />
        </TabsContent>
        <TabsContent value="providers">
          <ProvidersTab
            backend={backend.data}
            providers={providers.data ?? []}
            loading={providers.isLoading || backend.isLoading}
            onOpenQueue={(providerID) => {
              setProviderFilter(providerID);
              setTab("requests");
            }}
          />
        </TabsContent>
        <TabsContent value="cache">
          <CacheTab />
        </TabsContent>
        <TabsContent value="integrations">
          <IntegrationsTab />
        </TabsContent>
        <TabsContent value="delivery">
          <DeliveryTab />
        </TabsContent>
        <TabsContent value="settings">
          <SettingsTab
            backend={backend.data}
            providers={providers.data ?? []}
            loading={backend.isLoading || providers.isLoading}
          />
        </TabsContent>
      </Tabs>
    </div>
  );
}

function MetricCard({
  icon,
  label,
  value,
  detail,
}: {
  icon: React.ReactNode;
  label: string;
  value: string;
  detail: string;
}) {
  return (
    <Card className="gap-3 py-4">
      <CardHeader className="gap-1 px-4">
        <div className="flex items-center gap-2 text-sm text-muted-foreground">
          {icon}
          {label}
        </div>
        <CardTitle className="text-2xl">{value}</CardTitle>
        <CardDescription>{detail}</CardDescription>
      </CardHeader>
    </Card>
  );
}

function LibrariesTab({
  backend,
  libraries,
  backends,
  backendsError,
  loading,
}: {
  backend?: BackendConfig;
  libraries: LibraryInfo[];
  backends: BackendOption[];
  backendsError?: string | null;
  loading: boolean;
}) {
  const qc = useQueryClient();
  const [draft, setDraft] = useState<LibraryInfo[]>([]);
  // Only re-seed the editable draft when the server data actually changes
  // (e.g. after a save assigns ids, or another admin edits). A background
  // refetch (window refocus, refreshAll) returns identical data and must NOT
  // clobber the operator's in-progress, unsaved library edits.
  const syncedRef = useRef<string | null>(null);

  useEffect(() => {
    const snapshot = JSON.stringify(libraries);
    if (snapshot === syncedRef.current) return;
    syncedRef.current = snapshot;
    setDraft(
      libraries.map((lib, index) => ({
        ...lib,
        sort_order: lib.sort_order ?? index,
      })),
    );
  }, [libraries]);

  const save = useMutation({
    mutationFn: () =>
      adminReplaceLibraries(
        draft.map((lib, index) => ({
          ...lib,
          id: lib.id || 0,
          sort_order: index,
          name: lib.name.trim() || "Untitled",
          media_type: lib.media_type || "book",
        })),
      ),
    onSuccess: () => {
      toast.success("Libraries saved");
      qc.invalidateQueries({ queryKey: ["admin", "libraries"] });
      qc.invalidateQueries({ queryKey: ["admin", "backend"] });
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const [syncBackend, setSyncBackend] = useState<string>(
    backends[0] ? String(backends[0].id) : "",
  );
  const sync = useMutation({
    mutationFn: () => adminSyncLibraries(syncBackend),
    onSuccess: (r: {
      created: number;
      updated: number;
      pruned: number;
      kept: number;
    }) => {
      toast.success(
        `Synced: ${r.created} created, ${r.updated} updated, ${r.pruned} pruned`,
      );
      qc.invalidateQueries({ queryKey: ["admin", "libraries"] });
    },
    onError: (e: Error) => toast.error(e.message),
  });

  // backends loads async (React Query); seed the sync target once it arrives
  // so the button isn't stuck disabled on an available default.
  useEffect(() => {
    if (!syncBackend && backends[0]) {
      setSyncBackend(String(backends[0].id));
    }
  }, [backends, syncBackend]);

  const addLibrary = () =>
    setDraft((items) => [
      ...items,
      {
        id: 0,
        name: "New library",
        media_type: "book",
        backend_plugin_id: backends[0]
          ? String(backends[0].id)
          : backend?.target_backend_plugin_id || "",
        enabled: true,
        sort_order: items.length,
      },
    ]);

  const update = (index: number, patch: Partial<LibraryInfo>) =>
    setDraft((items) =>
      items.map((lib, i) => (i === index ? { ...lib, ...patch } : lib)),
    );

  const move = (index: number, direction: -1 | 1) => {
    setDraft((items) => {
      const next = [...items];
      const target = index + direction;
      if (target < 0 || target >= next.length) return items;
      [next[index], next[target]] = [next[target], next[index]];
      return next;
    });
  };

  const duplicate = (index: number) =>
    setDraft((items) => {
      const source = items[index];
      if (!source) return items;
      const copy = { ...source, id: 0, name: `${source.name} copy` };
      return [...items.slice(0, index + 1), copy, ...items.slice(index + 1)];
    });

  if (loading) return <Skeleton className="h-72 w-full" />;

  const hiddenLibraries = draft.filter((library) => !library.enabled).length;

  return (
    <div className="space-y-4">
      {backendsError && (
        <div className="rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">
          Couldn't load installed library sources: {backendsError}. The source
          dropdown below will be empty until this is resolved.
        </div>
      )}
      <div className="grid gap-3 md:grid-cols-3">
        <StatusPanel
          title="Visible libraries"
          value={String(draft.filter((library) => library.enabled).length)}
          detail="Shown in the reader navigation"
        />
        <StatusPanel
          title="Hidden libraries"
          value={String(hiddenLibraries)}
          detail="Configured but not user-visible"
        />
        <StatusPanel
          title="Catalog sources"
          value={String(backends.length)}
          detail="Available for library routing"
        />
      </div>

      <Card>
        <CardHeader className="border-b">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div>
              <CardTitle>Presentation libraries</CardTitle>
              <CardDescription>
                Presentation libraries are the top-level shelves users browse.
                Each one points to a catalog source and can optionally narrow to
                a source sub-library for comics, manga, magazines, or standard
                books.
              </CardDescription>
            </div>
            <div className="flex gap-2">
              <select
                className="h-9 rounded-md border border-border bg-background px-2 text-sm"
                value={syncBackend}
                onChange={(e) => setSyncBackend(e.target.value)}
                aria-label="Backend to sync from"
              >
                {backends.map((b) => (
                  <option key={b.id} value={String(b.id)}>
                    {b.display_name} ({b.plugin_id})
                  </option>
                ))}
              </select>
              <Button
                type="button"
                variant="outline"
                onClick={() => sync.mutate()}
                disabled={sync.isPending || !syncBackend}
              >
                Sync from backend
              </Button>
              <Button type="button" variant="outline" onClick={addLibrary}>
                Add library
              </Button>
              <Button
                type="button"
                onClick={() => save.mutate()}
                disabled={save.isPending}
              >
                <Save className="size-4" />
                Save libraries
              </Button>
            </div>
          </div>
        </CardHeader>
        <CardContent className="pt-5">
          {draft.length === 0 ? (
            <EmptyState
              icon={<LibraryBig className="size-7" />}
              title="No libraries configured"
              body="Create one or more user-facing libraries before exposing the reader portal. Download-only providers such as Anna's Archive do not appear here."
            />
          ) : (
            <div className="space-y-3">
              {draft.map((library, index) => (
                <LibraryEditorRow
                  key={`${library.id || "new"}-${index}`}
                  index={index}
                  total={draft.length}
                  library={library}
                  backends={backends}
                  onChange={(patch) => update(index, patch)}
                  onMove={move}
                  onDuplicate={() => duplicate(index)}
                  onRemove={() =>
                    setDraft((items) => items.filter((_, i) => i !== index))
                  }
                />
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      <BackendsPanel backends={backends} />
    </div>
  );
}

function LibraryEditorRow({
  index,
  total,
  library,
  backends,
  onChange,
  onMove,
  onDuplicate,
  onRemove,
}: {
  index: number;
  total: number;
  library: LibraryInfo;
  backends: BackendOption[];
  onChange: (patch: Partial<LibraryInfo>) => void;
  onMove: (index: number, direction: -1 | 1) => void;
  onDuplicate: () => void;
  onRemove: () => void;
}) {
  const backendID = library.backend_plugin_id ?? "";
  const backendLibraries = useQuery({
    queryKey: ["admin", "backend-libraries", backendID],
    queryFn: () => adminListBackendLibraries(backendID),
    enabled: !!backendID,
  });

  const selectedBackend = backends.find((b) => String(b.id) === backendID);

  return (
    <div className="rounded-lg border border-border bg-background p-4">
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <Badge variant={library.enabled ? "secondary" : "outline"}>
            {library.enabled ? "Enabled" : "Hidden"}
          </Badge>
          <span className="text-xs text-muted-foreground">
            Position {index + 1}
          </span>
          {selectedBackend && (
            <span className="text-xs text-muted-foreground">
              {selectedBackend.display_name} · {selectedBackend.plugin_id}
            </span>
          )}
        </div>
        <div className="flex flex-wrap gap-1.5">
          <Button
            type="button"
            size="sm"
            variant="ghost"
            disabled={index === 0}
            onClick={() => onMove(index, -1)}
          >
            Up
          </Button>
          <Button
            type="button"
            size="sm"
            variant="ghost"
            disabled={index === total - 1}
            onClick={() => onMove(index, 1)}
          >
            Down
          </Button>
          <Button type="button" size="sm" variant="ghost" onClick={onDuplicate}>
            Duplicate
          </Button>
          <Button type="button" size="sm" variant="ghost" onClick={onRemove}>
            <Trash2 className="size-4" />
            Remove
          </Button>
        </div>
      </div>
      <div className="grid gap-3 lg:grid-cols-[1.2fr_10rem_1.2fr_1.2fr_8rem]">
        <Field label="Display name">
          <Input
            value={library.name}
            onChange={(e) => onChange({ name: e.target.value })}
            placeholder="Books"
          />
        </Field>
        <Field label="Media type">
          <select
            value={library.media_type}
            onChange={(e) => onChange({ media_type: e.target.value })}
            className="h-9 w-full rounded-md border border-border bg-background px-3 text-sm"
          >
            <option value="book">Books</option>
            <option value="comic">Comics</option>
            <option value="manga">Manga</option>
            <option value="document">Documents</option>
            <option value="magazine">Magazines</option>
          </select>
        </Field>
        <Field label="Library source">
          <select
            value={backendID}
            onChange={(e) =>
              onChange({
                backend_plugin_id: e.target.value,
                backend_library_id: undefined,
              })
            }
            className="h-9 w-full rounded-md border border-border bg-background px-3 text-sm"
          >
            <option value="">Choose source</option>
            {backends.map((backend) => (
              <option key={backend.id} value={String(backend.id)}>
                {backend.display_name} ({backend.plugin_id})
              </option>
            ))}
          </select>
        </Field>
        <Field label="Source library">
          <select
            value={library.backend_library_id ?? ""}
            onChange={(e) =>
              onChange({
                backend_library_id: e.target.value
                  ? Number(e.target.value)
                  : undefined,
              })
            }
            className="h-9 w-full rounded-md border border-border bg-background px-3 text-sm"
            disabled={!backendID}
          >
            <option value="">All source items</option>
            {(backendLibraries.data?.items ?? []).map((lib) => (
              <option key={lib.id} value={lib.id}>
                {lib.name}
                {lib.media_type ? ` (${lib.media_type})` : ""}
              </option>
            ))}
          </select>
        </Field>
        <Field label="Visible">
          <label className="flex h-9 items-center gap-2 rounded-md border border-border px-3 text-sm">
            <input
              type="checkbox"
              checked={library.enabled}
              onChange={(e) => onChange({ enabled: e.target.checked })}
            />
            Enabled
          </label>
        </Field>
      </div>
    </div>
  );
}

function BackendsPanel({ backends }: { backends: BackendOption[] }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Available library sources</CardTitle>
        <CardDescription>
          These plugins can return catalog pages, book details, covers, and
          files for the reader UI. Request-only download providers are listed on
          the Providers tab instead.
        </CardDescription>
      </CardHeader>
      <CardContent>
        {backends.length === 0 ? (
          <EmptyState
            icon={<Database className="size-7" />}
            title="No library sources found"
            body="Install or enable a catalog-capable ebook source before routing presentation libraries."
          />
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Install ID</TableHead>
                <TableHead>Name</TableHead>
                <TableHead>Plugin ID</TableHead>
                <TableHead>Status</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {backends.map((backend) => (
                <TableRow key={backend.id}>
                  <TableCell>{backend.id}</TableCell>
                  <TableCell className="font-medium">
                    {backend.display_name}
                  </TableCell>
                  <TableCell>{backend.plugin_id}</TableCell>
                  <TableCell>
                    <Badge variant="secondary">Enabled</Badge>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </CardContent>
    </Card>
  );
}

function RequestsTab({
  providers,
  providerFilter,
  onProviderFilterChange,
}: {
  providers: BackendOption[];
  providerFilter: string;
  onProviderFilterChange: (providerID: string) => void;
}) {
  const [status, setStatus] = useState("");
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [activeRequest, setActiveRequest] = useState<Request | null>(null);
  const qc = useQueryClient();
  const requests = useQuery({
    queryKey: ["admin", "requests", status],
    queryFn: () => adminListRequests(status),
  });
  const patch = useMutation({
    mutationFn: ({
      id,
      action,
      denied_reason,
      fulfilled_book_id,
    }: {
      id: string;
      action: string;
      denied_reason?: string;
      fulfilled_book_id?: string;
    }) => adminPatchRequest(id, { action, denied_reason, fulfilled_book_id }),
    onSuccess: () => {
      toast.success("Request updated");
      qc.invalidateQueries({ queryKey: ["admin", "requests"] });
    },
    onError: (e: Error) => toast.error(e.message),
  });
  const bulk = useMutation({
    mutationFn: ({ action, ids }: { action: string; ids: string[] }) =>
      adminBulkRequests({ action, ids, denied_reason: "Bulk denied" }),
    onSuccess: (result) => {
      toast.success(`${result.updated} requests updated`);
      setSelected(new Set());
      qc.invalidateQueries({ queryKey: ["admin", "requests"] });
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const visibleRequests = (requests.data?.items ?? []).filter(
    (request) => !providerFilter || request.target_plugin_id === providerFilter,
  );
  const activeCount = visibleRequests.filter((request) =>
    ["pending", "submitted", "acknowledged", "downloading"].includes(
      request.status,
    ),
  ).length;
  const selectedIDs = visibleRequests
    .filter((request) => selected.has(request.id))
    .map((request) => request.id);
  const toggleSelected = (id: string) =>
    setSelected((current) => {
      const next = new Set(current);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });

  return (
    <div className="space-y-4">
      <div className="grid gap-3 md:grid-cols-3">
        <StatusPanel
          title="Visible queue"
          value={String(visibleRequests.length)}
          detail={
            providerFilter
              ? providerLabel(providers, providerFilter)
              : "All providers"
          }
        />
        <StatusPanel
          title="Needs attention"
          value={String(
            visibleRequests.filter((request) => request.status === "pending")
              .length,
          )}
          detail="Pending admin approval"
        />
        <StatusPanel
          title="In progress"
          value={String(activeCount)}
          detail="Submitted, acknowledged, or downloading"
        />
      </div>

      <Card>
        <CardHeader className="border-b">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div>
              <CardTitle>Request queue</CardTitle>
              <CardDescription>
                Review reader submissions before they are sent to a download
                provider. Approval publishes a request event to the selected
                provider; fulfillment is recorded when that provider reports a
                completed file or when an admin links one manually.
              </CardDescription>
            </div>
            <div className="flex flex-wrap gap-2">
              {selectedIDs.length > 0 && (
                <>
                  <Button
                    type="button"
                    size="sm"
                    disabled={bulk.isPending}
                    onClick={() =>
                      bulk.mutate({ action: "approve", ids: selectedIDs })
                    }
                  >
                    Approve selected
                  </Button>
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    disabled={bulk.isPending}
                    onClick={() =>
                      bulk.mutate({ action: "retry", ids: selectedIDs })
                    }
                  >
                    Retry selected
                  </Button>
                </>
              )}
              <select
                value={providerFilter}
                onChange={(e) => onProviderFilterChange(e.target.value)}
                className="h-9 rounded-md border border-border bg-background px-3 text-sm"
              >
                <option value="">All providers</option>
                {providers.map((provider) => (
                  <option key={provider.id} value={String(provider.id)}>
                    {provider.display_name}
                  </option>
                ))}
              </select>
              <select
                value={status}
                onChange={(e) => setStatus(e.target.value)}
                className="h-9 rounded-md border border-border bg-background px-3 text-sm"
              >
                {requestStatuses.map((s) => (
                  <option key={s || "active"} value={s}>
                    {s ? titleCase(s) : "Active queue"}
                  </option>
                ))}
              </select>
            </div>
          </div>
        </CardHeader>
        <CardContent className="pt-5">
          {requests.isLoading ? (
            <Skeleton className="h-56 w-full" />
          ) : !visibleRequests.length ? (
            <EmptyState
              icon={<Send className="size-7" />}
              title="No requests"
              body="There are no requests matching the selected filter."
            />
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-10"></TableHead>
                  <TableHead>Title</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Download provider</TableHead>
                  <TableHead>Created</TableHead>
                  <TableHead className="text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {visibleRequests.map((request) => (
                  <RequestRow
                    key={request.id}
                    request={request}
                    providers={providers}
                    selected={selected.has(request.id)}
                    onSelect={() => toggleSelected(request.id)}
                    disabled={patch.isPending}
                    onApprove={() =>
                      patch.mutate({ id: request.id, action: "approve" })
                    }
                    onOpen={() => setActiveRequest(request)}
                  />
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
      <RequestDetailDialog
        request={activeRequest}
        providers={providers}
        disabled={patch.isPending}
        onOpenChange={(open) => {
          if (!open) setActiveRequest(null);
        }}
        onAction={(body) => {
          if (!activeRequest) return;
          patch.mutate({ id: activeRequest.id, ...body });
          setActiveRequest(null);
        }}
      />
    </div>
  );
}

function RequestRow({
  request,
  providers,
  selected,
  onSelect,
  disabled,
  onApprove,
  onOpen,
}: {
  request: Request;
  providers: BackendOption[];
  selected: boolean;
  onSelect: () => void;
  disabled: boolean;
  onApprove: () => void;
  onOpen: () => void;
}) {
  return (
    <TableRow>
      <TableCell>
        <input
          type="checkbox"
          checked={selected}
          onChange={onSelect}
          aria-label={`Select ${request.title}`}
        />
      </TableCell>
      <TableCell>
        <div className="max-w-md">
          <div className="truncate font-medium">{request.title}</div>
          <div className="truncate text-xs text-muted-foreground">
            {(request.authors ?? []).join(", ") ||
              request.isbn ||
              request.source_id ||
              request.id}
          </div>
          <div className="mt-1 flex flex-wrap gap-1.5 text-xs text-muted-foreground">
            <Badge variant="outline">{request.media_type || "book"}</Badge>
            {request.format_pref && (
              <Badge variant="outline">
                {request.format_pref.toUpperCase()}
              </Badge>
            )}
            {request.source_id && (
              <Badge variant="outline">Source {request.source_id}</Badge>
            )}
            {request.external_id && (
              <Badge variant="outline">External {request.external_id}</Badge>
            )}
            {request.failure_reason && (
              <span className="text-destructive">{request.failure_reason}</span>
            )}
            {request.denied_reason && (
              <span className="text-destructive">{request.denied_reason}</span>
            )}
          </div>
        </div>
      </TableCell>
      <TableCell>
        <StatusBadge status={request.status} />
      </TableCell>
      <TableCell>
        <div className="max-w-xs">
          <div className="font-medium">
            {providerLabel(providers, request.target_plugin_id)}
          </div>
          {request.target_plugin_id && (
            <div className="text-xs text-muted-foreground">
              Install {request.target_plugin_id}
            </div>
          )}
        </div>
      </TableCell>
      <TableCell>{formatDate(request.created_at)}</TableCell>
      <TableCell>
        <div className="flex justify-end gap-1.5">
          {request.status === "pending" && (
            <>
              <Button size="sm" onClick={onApprove} disabled={disabled}>
                Approve
              </Button>
              <Button
                size="sm"
                variant="ghost"
                onClick={onOpen}
                disabled={disabled}
              >
                Deny
              </Button>
            </>
          )}
          {!["fulfilled", "denied"].includes(request.status) && (
            <Button
              size="sm"
              variant="outline"
              onClick={onOpen}
              disabled={disabled}
            >
              Details
            </Button>
          )}
          {["failed", "submitted", "acknowledged", "downloading"].includes(
            request.status,
          ) && (
            <Button
              size="sm"
              variant="ghost"
              onClick={onApprove}
              disabled={disabled}
            >
              Retry
            </Button>
          )}
        </div>
      </TableCell>
    </TableRow>
  );
}

function RequestDetailDialog({
  request,
  providers,
  disabled,
  onOpenChange,
  onAction,
}: {
  request: Request | null;
  providers: BackendOption[];
  disabled: boolean;
  onOpenChange: (open: boolean) => void;
  onAction: (body: {
    action: string;
    denied_reason?: string;
    fulfilled_book_id?: string;
  }) => void;
}) {
  const [deniedReason, setDeniedReason] = useState("");
  const [fulfilledBookID, setFulfilledBookID] = useState("");

  useEffect(() => {
    setDeniedReason(request?.denied_reason || "");
    setFulfilledBookID(request?.fulfilled_book_id || "");
  }, [request]);

  return (
    <Dialog open={!!request} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>{request?.title || "Request"}</DialogTitle>
          <DialogDescription>
            Full request state, provider routing, and manual admin actions.
          </DialogDescription>
        </DialogHeader>
        {request && (
          <div className="grid gap-4 lg:grid-cols-[1fr_18rem]">
            <div className="space-y-3">
              <div className="rounded-lg border border-border p-3">
                <div className="mb-2 flex flex-wrap gap-1.5">
                  <StatusBadge status={request.status} />
                  <Badge variant="outline">
                    {request.media_type || "book"}
                  </Badge>
                  {request.format_pref && (
                    <Badge variant="outline">
                      {request.format_pref.toUpperCase()}
                    </Badge>
                  )}
                </div>
                <DetailGrid
                  rows={[
                    ["User", request.user_id],
                    ["Authors", (request.authors ?? []).join(", ")],
                    ["ISBN", request.isbn],
                    ["Source ID", request.source_id],
                    ["External ID", request.external_id],
                    [
                      "Provider",
                      providerLabel(providers, request.target_plugin_id),
                    ],
                    ["Install ID", request.target_plugin_id],
                    ["Created", formatDate(request.created_at)],
                    ["Updated", formatDate(request.updated_at)],
                    ["Fulfilled", formatDate(request.fulfilled_at)],
                  ]}
                />
              </div>
              <div className="rounded-lg border border-border p-3">
                <div className="mb-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">
                  Lifecycle
                </div>
                <div className="space-y-2 text-sm">
                  <TimelineItem
                    label="Submitted"
                    value={formatDate(request.created_at)}
                  />
                  <TimelineItem
                    label={titleCase(request.status)}
                    value={formatDate(request.updated_at)}
                  />
                  {request.failure_reason && (
                    <TimelineItem
                      label="Failure"
                      value={request.failure_reason}
                    />
                  )}
                  {request.denied_reason && (
                    <TimelineItem
                      label="Denied"
                      value={request.denied_reason}
                    />
                  )}
                  {request.fulfilled_book_id && (
                    <TimelineItem
                      label="Book"
                      value={request.fulfilled_book_id}
                    />
                  )}
                </div>
              </div>
            </div>
            <div className="space-y-3">
              <div className="rounded-lg border border-border p-3">
                <label className="mb-1 block text-xs font-medium text-muted-foreground">
                  Manual fulfilled book ID
                </label>
                <Input
                  value={fulfilledBookID}
                  onChange={(e) => setFulfilledBookID(e.target.value)}
                  placeholder="library:encoded-book-id"
                />
                <Button
                  className="mt-2 w-full"
                  disabled={disabled || !fulfilledBookID.trim()}
                  onClick={() =>
                    onAction({
                      action: "fulfill_manual",
                      fulfilled_book_id: fulfilledBookID.trim(),
                    })
                  }
                >
                  Mark fulfilled
                </Button>
              </div>
              <div className="rounded-lg border border-border p-3">
                <label className="mb-1 block text-xs font-medium text-muted-foreground">
                  Denial reason
                </label>
                <textarea
                  value={deniedReason}
                  onChange={(e) => setDeniedReason(e.target.value)}
                  className="min-h-24 w-full rounded-md border border-border bg-background px-3 py-2 text-sm"
                />
                <Button
                  className="mt-2 w-full"
                  variant="destructive"
                  disabled={disabled || !deniedReason.trim()}
                  onClick={() =>
                    onAction({
                      action: "deny",
                      denied_reason: deniedReason.trim(),
                    })
                  }
                >
                  Deny request
                </Button>
              </div>
            </div>
          </div>
        )}
        <DialogFooter>
          {request?.status === "pending" && (
            <Button
              disabled={disabled}
              onClick={() => onAction({ action: "approve" })}
            >
              Approve
            </Button>
          )}
          {request &&
            ["failed", "submitted", "acknowledged", "downloading"].includes(
              request.status,
            ) && (
              <Button
                disabled={disabled}
                variant="outline"
                onClick={() => onAction({ action: "retry" })}
              >
                Retry
              </Button>
            )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function DetailGrid({ rows }: { rows: [string, React.ReactNode][] }) {
  return (
    <dl className="grid gap-2 text-sm sm:grid-cols-2">
      {rows
        .filter(([, value]) => value !== undefined && value !== "")
        .map(([label, value]) => (
          <div key={label}>
            <dt className="text-xs text-muted-foreground">{label}</dt>
            <dd className="break-words font-medium">{value}</dd>
          </div>
        ))}
    </dl>
  );
}

function TimelineItem({
  label,
  value,
}: {
  label: string;
  value: React.ReactNode;
}) {
  return (
    <div className="flex gap-3">
      <div className="mt-1 size-2 rounded-full bg-primary" />
      <div>
        <div className="font-medium">{label}</div>
        <div className="text-xs text-muted-foreground">{value}</div>
      </div>
    </div>
  );
}

function StatusPanel({
  title,
  value,
  detail,
}: {
  title: string;
  value: string;
  detail: string;
}) {
  return (
    <div className="rounded-lg border border-border bg-background p-4">
      <div className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
        {title}
      </div>
      <div className="mt-2 text-2xl font-semibold">{value}</div>
      <div className="mt-1 text-sm text-muted-foreground">{detail}</div>
    </div>
  );
}

function ProvidersTab({
  backend,
  providers,
  loading,
  onOpenQueue,
}: {
  backend?: BackendConfig;
  providers: BackendOption[];
  loading: boolean;
  onOpenQueue: (providerID: string) => void;
}) {
  const qc = useQueryClient();
  const activityQueries = useQueries({
    queries: trackedRequestStatuses.map((status) => ({
      queryKey: ["admin", "requests", "providers", status],
      queryFn: () => adminListRequests(status),
    })),
  });
  const healthQueries = useQueries({
    queries: providers.map((provider) => ({
      queryKey: ["admin", "providers", provider.id, "health"],
      queryFn: () => adminProviderHealth(String(provider.id)),
    })),
  });
  const setDefault = useMutation({
    mutationFn: (providerID: string) =>
      adminPatchBackend({ target_backend_plugin_id: providerID }),
    onSuccess: () => {
      toast.success("Default provider updated");
      qc.invalidateQueries({ queryKey: ["admin", "backend"] });
    },
    onError: (e: Error) => toast.error(e.message),
  });
  const testSearch = useMutation({
    mutationFn: ({ providerID, q }: { providerID: string; q: string }) =>
      adminProviderTestSearch(providerID, q),
    onSuccess: (result) => {
      if (result.ok) {
        toast.success(`Provider search returned ${result.items.length} items`);
      } else {
        toast.error(result.message);
      }
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const allProviderRequests = activityQueries.flatMap(
    (query) => query.data?.items ?? [],
  );
  const activityLoading =
    loading || activityQueries.some((query) => query.isLoading);
  const defaultProviderID = backend?.target_backend_plugin_id || "";
  const providerIDs = new Set(providers.map((provider) => String(provider.id)));
  const providerRequestCount = allProviderRequests.filter((request) =>
    providerIDs.has(request.target_plugin_id),
  ).length;
  const activeProviderRequests = allProviderRequests.filter(
    (request) =>
      providerIDs.has(request.target_plugin_id) &&
      ["pending", "submitted", "acknowledged", "downloading"].includes(
        request.status,
      ),
  ).length;

  if (activityLoading) return <Skeleton className="h-72 w-full" />;

  return (
    <div className="space-y-4">
      <div className="grid gap-3 md:grid-cols-3">
        <StatusPanel
          title="Enabled providers"
          value={String(providers.length)}
          detail={
            defaultProviderID
              ? `Default: ${providerLabel(providers, defaultProviderID)}`
              : "No default selected"
          }
        />
        <StatusPanel
          title="Tracked requests"
          value={String(providerRequestCount)}
          detail="Provider-targeted queue history"
        />
        <StatusPanel
          title="Active work"
          value={String(activeProviderRequests)}
          detail="Pending, submitted, acknowledged, or downloading"
        />
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Download providers</CardTitle>
          <CardDescription>
            Download providers are acquisition targets for requests. They are
            separate from presentation libraries: a provider can download or
            monitor a request without being a browsable source in the reader UI.
          </CardDescription>
        </CardHeader>
        <CardContent>
          {providers.length === 0 ? (
            <EmptyState
              icon={<Send className="size-7" />}
              title="No download providers found"
              body="Enable an ebook download provider before users submit acquisition requests."
            />
          ) : (
            <div className="grid gap-3 xl:grid-cols-2">
              {providers.map((provider, index) => {
                const profile = providerProfile(provider);
                const providerID = String(provider.id);
                const providerRequests = allProviderRequests
                  .filter((request) => request.target_plugin_id === providerID)
                  .sort(
                    (a, b) =>
                      new Date(b.updated_at || b.created_at || 0).getTime() -
                      new Date(a.updated_at || a.created_at || 0).getTime(),
                  );
                const active = providerRequests.filter((request) =>
                  [
                    "pending",
                    "submitted",
                    "acknowledged",
                    "downloading",
                  ].includes(request.status),
                );
                const failed = providerRequests.filter(
                  (request) => request.status === "failed",
                );
                const fulfilled = providerRequests.filter(
                  (request) => request.status === "fulfilled",
                );
                const latest = providerRequests[0];
                const isDefault = providerID === defaultProviderID;
                const health = healthQueries[index]?.data;
                return (
                  <div
                    key={provider.id}
                    className="rounded-lg border border-border bg-background p-4"
                  >
                    <div className="mb-3 flex flex-wrap items-start justify-between gap-3">
                      <div>
                        <h3 className="font-semibold">
                          {provider.display_name}
                        </h3>
                        <p className="text-xs text-muted-foreground">
                          Install {provider.id} · {provider.plugin_id}
                        </p>
                      </div>
                      <div className="flex flex-wrap gap-1.5">
                        {isDefault && (
                          <Badge variant="secondary">Default</Badge>
                        )}
                        {health && (
                          <Badge
                            variant={health.ok ? "secondary" : "destructive"}
                          >
                            {health.ok ? "Healthy" : "Unavailable"}
                          </Badge>
                        )}
                        <Badge variant="outline">{profile.role}</Badge>
                      </div>
                    </div>
                    <div className="mb-4 grid grid-cols-4 gap-2">
                      <ProviderStat label="Active" value={active.length} />
                      <ProviderStat label="Done" value={fulfilled.length} />
                      <ProviderStat label="Failed" value={failed.length} />
                      <ProviderStat
                        label="Latest"
                        value={latest ? formatDate(latest.updated_at) : "Never"}
                      />
                    </div>
                    <div className="space-y-3 text-sm">
                      <DescriptionBlock title="Health">
                        {health
                          ? health.message
                          : "Health check has not completed yet."}
                        {health?.formats?.length
                          ? ` Formats: ${health.formats.join(", ")}.`
                          : ""}
                      </DescriptionBlock>
                      <DescriptionBlock title="What it does">
                        {profile.summary}
                      </DescriptionBlock>
                      <DescriptionBlock title="Best for">
                        {profile.bestFor}
                      </DescriptionBlock>
                      <DescriptionBlock title="Requirements">
                        {profile.requirements}
                      </DescriptionBlock>
                    </div>
                    <div className="mt-4 flex flex-wrap gap-2">
                      <Button
                        type="button"
                        size="sm"
                        disabled={isDefault || setDefault.isPending}
                        onClick={() => setDefault.mutate(providerID)}
                      >
                        {isDefault ? "Current default" : "Make default"}
                      </Button>
                      <Button
                        type="button"
                        size="sm"
                        variant="outline"
                        onClick={() => onOpenQueue(providerID)}
                      >
                        View queue
                      </Button>
                      <Button
                        type="button"
                        size="sm"
                        variant="outline"
                        disabled={testSearch.isPending}
                        onClick={() =>
                          testSearch.mutate({
                            providerID,
                            q: latest?.title || "foundation",
                          })
                        }
                      >
                        Test search
                      </Button>
                    </div>
                    {testSearch.variables?.providerID === providerID &&
                      testSearch.data && (
                        <div className="mt-3 rounded-md border border-border bg-muted/30 p-3 text-sm">
                          <div className="font-medium">
                            {testSearch.data.message}
                          </div>
                          {(testSearch.data.items ?? []).length > 0 && (
                            <div className="mt-2 space-y-1">
                              {(testSearch.data.items ?? []).map((item) => (
                                <div
                                  key={item.id}
                                  className="truncate text-xs text-muted-foreground"
                                >
                                  {item.title}
                                  {item.authors?.length
                                    ? ` · ${item.authors.join(", ")}`
                                    : ""}
                                </div>
                              ))}
                            </div>
                          )}
                        </div>
                      )}
                    <div className="mt-4 border-t pt-3">
                      <div className="mb-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">
                        Recent activity
                      </div>
                      {providerRequests.length === 0 ? (
                        <p className="text-sm text-muted-foreground">
                          No requests have targeted this provider yet.
                        </p>
                      ) : (
                        <div className="space-y-2">
                          {providerRequests.slice(0, 3).map((request) => (
                            <div
                              key={request.id}
                              className="flex items-center justify-between gap-3 rounded-md bg-muted/40 px-3 py-2 text-sm"
                            >
                              <div className="min-w-0">
                                <div className="truncate font-medium">
                                  {request.title}
                                </div>
                                <div className="text-xs text-muted-foreground">
                                  {formatDate(
                                    request.updated_at || request.created_at,
                                  )}
                                </div>
                              </div>
                              <StatusBadge status={request.status} />
                            </div>
                          ))}
                        </div>
                      )}
                    </div>
                  </div>
                );
              })}
            </div>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Provider routing rules</CardTitle>
          <CardDescription>
            Send different media types to different acquisition providers. Users
            can still override the provider manually when needed.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <RoutingRulesEditor
            providers={providers}
            defaultProviderID={defaultProviderID}
          />
        </CardContent>
      </Card>
    </div>
  );
}

function RoutingRulesEditor({
  providers,
  defaultProviderID,
}: {
  providers: BackendOption[];
  defaultProviderID: string;
}) {
  const qc = useQueryClient();
  const rules = useQuery({
    queryKey: ["admin", "routing-rules"],
    queryFn: adminListRoutingRules,
  });
  const [draft, setDraft] = useState<RequestRoutingRule[]>([]);
  useEffect(() => {
    setDraft(rules.data?.items ?? []);
  }, [rules.data?.items]);
  const save = useMutation({
    mutationFn: () => adminReplaceRoutingRules(draft),
    onSuccess: () => {
      toast.success("Routing rules saved");
      qc.invalidateQueries({ queryKey: ["admin", "routing-rules"] });
    },
    onError: (e: Error) => toast.error(e.message),
  });
  const addRule = () =>
    setDraft((current) => [
      ...current,
      {
        id: 0,
        media_type: "comic",
        target_plugin_id: defaultProviderID || String(providers[0]?.id ?? ""),
        format_pref: "epub",
        auto_monitor: false,
        enabled: true,
        sort_order: current.length,
      },
    ]);
  const mediaTypes = draft
    .map((rule) => rule.media_type.trim())
    .filter(Boolean);
  const duplicateMediaTypes = new Set(
    mediaTypes.filter(
      (mediaType, index) => mediaTypes.indexOf(mediaType) !== index,
    ),
  );
  const hasInvalidRules =
    duplicateMediaTypes.size > 0 ||
    draft.some((rule) => !rule.media_type.trim() || !rule.target_plugin_id);

  if (rules.isLoading) return <Skeleton className="h-28 w-full" />;

  return (
    <div className="space-y-3">
      {draft.length === 0 ? (
        <EmptyState
          icon={<Send className="size-7" />}
          title="No routing rules"
          body="Without rules, automatic requests use the global default provider."
        />
      ) : (
        <div className="space-y-2">
          {draft.map((rule, index) => (
            <div
              key={`${rule.id}-${index}`}
              className="grid gap-2 rounded-lg border border-border p-3 lg:grid-cols-[1fr_1.5fr_1fr_auto_auto]"
            >
              <Input
                value={rule.media_type}
                onChange={(e) =>
                  setDraft((current) =>
                    current.map((item, i) =>
                      i === index
                        ? { ...item, media_type: e.target.value }
                        : item,
                    ),
                  )
                }
                placeholder="book, comic, manga"
              />
              <select
                value={rule.target_plugin_id}
                onChange={(e) =>
                  setDraft((current) =>
                    current.map((item, i) =>
                      i === index
                        ? { ...item, target_plugin_id: e.target.value }
                        : item,
                    ),
                  )
                }
                className="h-9 rounded-md border border-border bg-background px-3 text-sm"
              >
                {providers.map((provider) => (
                  <option key={provider.id} value={String(provider.id)}>
                    {provider.display_name}
                  </option>
                ))}
              </select>
              <select
                value={rule.format_pref || ""}
                onChange={(e) =>
                  setDraft((current) =>
                    current.map((item, i) =>
                      i === index
                        ? { ...item, format_pref: e.target.value }
                        : item,
                    ),
                  )
                }
                className="h-9 rounded-md border border-border bg-background px-3 text-sm"
              >
                <option value="">User choice</option>
                <option value="epub">EPUB</option>
                <option value="pdf">PDF</option>
                <option value="cbz">CBZ</option>
                <option value="cbr">CBR</option>
              </select>
              <label className="flex h-9 items-center gap-2 rounded-md border border-border px-3 text-sm">
                <input
                  type="checkbox"
                  checked={rule.enabled}
                  onChange={(e) =>
                    setDraft((current) =>
                      current.map((item, i) =>
                        i === index
                          ? { ...item, enabled: e.target.checked }
                          : item,
                      ),
                    )
                  }
                />
                Enabled
              </label>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                onClick={() =>
                  setDraft((current) => current.filter((_, i) => i !== index))
                }
              >
                <Trash2 className="size-4" />
              </Button>
            </div>
          ))}
        </div>
      )}
      <div className="flex flex-wrap gap-2">
        {duplicateMediaTypes.size > 0 && (
          <div className="basis-full text-sm text-destructive">
            Duplicate media types: {Array.from(duplicateMediaTypes).join(", ")}
          </div>
        )}
        <Button type="button" variant="outline" onClick={addRule}>
          Add rule
        </Button>
        <Button
          type="button"
          onClick={() => save.mutate()}
          disabled={save.isPending || providers.length === 0 || hasInvalidRules}
        >
          <Save className="size-4" />
          Save routing
        </Button>
      </div>
    </div>
  );
}

function ProviderStat({
  label,
  value,
}: {
  label: string;
  value: React.ReactNode;
}) {
  return (
    <div className="rounded-md border border-border px-3 py-2">
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="mt-1 truncate text-sm font-semibold">{value}</div>
    </div>
  );
}

function DescriptionBlock({
  title,
  children,
}: {
  title: string;
  children: React.ReactNode;
}) {
  return (
    <div>
      <div className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
        {title}
      </div>
      <p className="mt-1 leading-6 text-foreground/85">{children}</p>
    </div>
  );
}

function CacheTab() {
  const cache = useQuery({
    queryKey: ["admin", "cache"],
    queryFn: adminCacheStats,
  });
  const backend = useQuery({
    queryKey: ["admin", "backend"],
    queryFn: adminGetBackend,
  });
  const largest = useQuery({
    queryKey: ["admin", "cache", "largest"],
    queryFn: adminCacheLargest,
  });
  const used = cache.data?.bytes_used ?? 0;
  const max = cache.data?.bytes_max ?? 0;
  const pct = max ? (used / max) * 100 : 0;
  const largestCacheEntries = largest.data?.items ?? [];

  return (
    <div className="space-y-4">
      <div className="grid gap-3 md:grid-cols-3">
        <StatusPanel
          title="Streaming mode"
          value={backend.data?.default_streaming_mode || "proxy"}
          detail={
            backend.data?.default_streaming_mode === "cache"
              ? "Files are saved locally before serving"
              : "Files stream directly from the source"
          }
        />
        <StatusPanel
          title="Cache limit"
          value={formatBytes(max)}
          detail={`${pct.toFixed(1)}% currently used`}
        />
        <StatusPanel
          title="Download workers"
          value={String(backend.data?.cache_download_concurrency ?? 0)}
          detail="Parallel cache fill limit"
        />
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Cache usage</CardTitle>
          <CardDescription>
            Shows local disk use for files saved by cache streaming mode. Proxy
            streaming does not persist a local copy here.
          </CardDescription>
        </CardHeader>
        <CardContent>
          {cache.isLoading ? (
            <Skeleton className="h-20 w-full" />
          ) : (
            <>
              <div className="mb-2 flex items-baseline justify-between gap-3">
                <span className="text-2xl font-semibold">
                  {formatBytes(used)}
                </span>
                <span className="text-sm text-muted-foreground">
                  {pct.toFixed(1)}% of {formatBytes(max)}
                </span>
              </div>
              <div className="h-2 overflow-hidden rounded-full bg-muted">
                <div
                  className="h-full bg-primary"
                  style={{ width: `${Math.min(100, pct)}%` }}
                />
              </div>
            </>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Largest cached files</CardTitle>
          <CardDescription>
            Largest ready cache entries by on-disk size, useful when deciding
            whether to raise the cache limit or let eviction reclaim space.
          </CardDescription>
        </CardHeader>
        <CardContent>
          {largest.isLoading ? (
            <Skeleton className="h-56 w-full" />
          ) : largestCacheEntries.length === 0 ? (
            <EmptyState
              icon={<HardDrive className="size-7" />}
              title="No cached files"
              body="Cache entries appear after books are served with cache streaming mode enabled."
            />
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Book</TableHead>
                  <TableHead>Format</TableHead>
                  <TableHead>Size</TableHead>
                  <TableHead>Last access</TableHead>
                  <TableHead>Status</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {largestCacheEntries.map((entry) => (
                  <TableRow key={text(entry, "id", "ID")}>
                    <TableCell className="max-w-sm truncate">
                      {text(entry, "book_id", "BookID")}
                    </TableCell>
                    <TableCell>{text(entry, "format", "Format")}</TableCell>
                    <TableCell>
                      {formatBytes(
                        number(entry, "bytes_on_disk", "BytesOnDisk"),
                      )}
                    </TableCell>
                    <TableCell>
                      {formatDate(
                        text(entry, "last_accessed_at", "LastAccessedAt"),
                      )}
                    </TableCell>
                    <TableCell>
                      <StatusBadge
                        status={text(entry, "status", "Status") || "ready"}
                      />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function IntegrationsTab() {
  const qc = useQueryClient();
  const opds = useQuery({
    queryKey: ["admin", "opds-tokens"],
    queryFn: adminOPDSTokens,
  });
  const kosync = useQuery({
    queryKey: ["admin", "kosync-users"],
    queryFn: adminKosyncUsers,
  });
  const kobo = useQuery({
    queryKey: ["admin", "kobo-sessions"],
    queryFn: adminKoboSessions,
  });
  const revokeToken = useMutation({
    mutationFn: adminRevokeOPDSToken,
    onSuccess: () => {
      toast.success("OPDS token revoked");
      qc.invalidateQueries({ queryKey: ["admin", "opds-tokens"] });
    },
    onError: (e: Error) => toast.error(e.message),
  });
  const deleteKosync = useMutation({
    mutationFn: adminDeleteKosyncUser,
    onSuccess: () => {
      toast.success("KOReader sync user deleted");
      qc.invalidateQueries({ queryKey: ["admin", "kosync-users"] });
    },
    onError: (e: Error) => toast.error(e.message),
  });

  return (
    <div className="space-y-4">
      <div className="grid gap-3 md:grid-cols-3">
        <StatusPanel
          title="OPDS tokens"
          value={String(
            (opds.data?.items ?? []).filter(
              (token) => !text(token, "revoked_at", "RevokedAt"),
            ).length,
          )}
          detail={`${(opds.data?.items ?? []).length} total issued`}
        />
        <StatusPanel
          title="KOReader users"
          value={String((kosync.data?.items ?? []).length)}
          detail="Registered sync identities"
        />
        <StatusPanel
          title="Kobo sessions"
          value={String(
            (kobo.data?.items ?? []).filter(
              (session) => text(session, "status", "Status") === "active",
            ).length,
          )}
          detail={`${(kobo.data?.items ?? []).length} recent transfers`}
        />
      </div>

      <div className="grid gap-4 xl:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>OPDS tokens</CardTitle>
            <CardDescription>
              Per-user feed credentials for OPDS-capable readers. Revoking a
              token immediately blocks that reader without changing the user's
              Continuum account.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <DataTable
              loading={opds.isLoading}
              emptyIcon={<KeyRound className="size-7" />}
              emptyTitle="No OPDS tokens"
              headers={["User", "Label", "Last used", "Status", ""]}
              rows={(opds.data?.items ?? []).map((token) => [
                text(token, "user_id", "UserID"),
                text(token, "label", "Label") || "Reader",
                formatDate(text(token, "last_used_at", "LastUsedAt")),
                <StatusBadge
                  key="status"
                  status={
                    text(token, "revoked_at", "RevokedAt")
                      ? "revoked"
                      : "active"
                  }
                />,
                <Button
                  key="action"
                  size="sm"
                  variant="ghost"
                  disabled={
                    !!text(token, "revoked_at", "RevokedAt") ||
                    revokeToken.isPending
                  }
                  onClick={() => revokeToken.mutate(text(token, "id", "ID"))}
                >
                  Revoke
                </Button>,
              ])}
            />
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>KOReader sync users</CardTitle>
            <CardDescription>
              KOReader progress-sync accounts bound to Continuum users. Removing
              one disables sync for that reader profile.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <DataTable
              loading={kosync.isLoading}
              emptyIcon={<RefreshCw className="size-7" />}
              emptyTitle="No KOReader sync users"
              headers={["Username", "User ID", "Created", ""]}
              rows={(kosync.data?.items ?? []).map((user) => [
                text(user, "kosync_username", "KosyncUsername"),
                text(user, "user_id", "UserID"),
                formatDate(text(user, "created_at", "CreatedAt")),
                <Button
                  key="delete"
                  size="sm"
                  variant="ghost"
                  disabled={deleteKosync.isPending}
                  onClick={() =>
                    deleteKosync.mutate(
                      text(user, "kosync_username", "KosyncUsername"),
                    )
                  }
                >
                  Delete
                </Button>,
              ])}
            />
          </CardContent>
        </Card>

        <Card className="xl:col-span-2">
          <CardHeader>
            <CardTitle>Kobo transfer sessions</CardTitle>
            <CardDescription>
              One-time transfer links created for Kobo devices. These sessions
              expire automatically and show whether the device completed the
              handoff.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <DataTable
              loading={kobo.isLoading}
              emptyIcon={<Smartphone className="size-7" />}
              emptyTitle="No Kobo sessions"
              headers={[
                "User",
                "Book",
                "Format",
                "Status",
                "Expires",
                "Completed",
              ]}
              rows={(kobo.data?.items ?? []).map((session) => [
                text(session, "user_id", "UserID"),
                text(session, "book_id", "BookID"),
                text(session, "format", "Format"),
                <StatusBadge
                  key="status"
                  status={text(session, "status", "Status")}
                />,
                formatDate(text(session, "expires_at", "ExpiresAt")),
                formatDate(text(session, "completed_at", "CompletedAt")),
              ])}
            />
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

function DeliveryTab() {
  const kindle = useQuery({
    queryKey: ["admin", "kindle-log"],
    queryFn: adminKindleLog,
  });
  const sends = kindle.data?.items ?? [];
  const queued = sends.filter(
    (send) => text(send, "status", "Status") === "queued",
  ).length;
  const sent = sends.filter(
    (send) => text(send, "status", "Status") === "sent",
  ).length;
  const failed = sends.filter(
    (send) => text(send, "status", "Status") === "failed",
  ).length;

  return (
    <div className="space-y-4">
      <div className="grid gap-3 md:grid-cols-3">
        <StatusPanel
          title="Queued"
          value={String(queued)}
          detail="Waiting for send retry"
        />
        <StatusPanel
          title="Sent"
          value={String(sent)}
          detail="Delivered to SMTP"
        />
        <StatusPanel
          title="Failed"
          value={String(failed)}
          detail="Needs configuration or address review"
        />
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Kindle delivery log</CardTitle>
          <CardDescription>
            Email delivery attempts for Send-to-Kindle. Failures usually
            indicate SMTP configuration, rejected recipient addresses, or format
            conversion problems.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <DataTable
            loading={kindle.isLoading}
            emptyIcon={<Mail className="size-7" />}
            emptyTitle="No Kindle sends"
            headers={[
              "User",
              "Book",
              "Format",
              "Address",
              "Status",
              "Created",
              "Sent/Error",
            ]}
            rows={sends.map((send) => [
              text(send, "user_id", "UserID"),
              text(send, "book_id", "BookID"),
              text(send, "format", "Format"),
              text(send, "to_address", "ToAddress"),
              <StatusBadge
                key="status"
                status={text(send, "status", "Status")}
              />,
              formatDate(text(send, "created_at", "CreatedAt")),
              text(send, "error_text", "ErrorText") ||
                formatDate(text(send, "sent_at", "SentAt")),
            ])}
          />
        </CardContent>
      </Card>
    </div>
  );
}

function SettingsTab({
  backend,
  providers,
  loading,
}: {
  backend?: BackendConfig;
  providers: BackendOption[];
  loading: boolean;
}) {
  const qc = useQueryClient();
  const [draft, setDraft] = useState<Partial<BackendConfig>>({});
  const [pathRemappingsText, setPathRemappingsText] = useState("[]");
  const [kindleSMTPText, setKindleSMTPText] = useState("{}");

  useEffect(() => {
    if (backend) {
      setDraft(backend);
      setPathRemappingsText(
        JSON.stringify(backend.path_remappings ?? [], null, 2),
      );
      setKindleSMTPText(
        JSON.stringify(backend.kindle_smtp_config ?? {}, null, 2),
      );
    }
  }, [backend]);

  const save = useMutation({
    mutationFn: () => {
      let path_remappings: unknown[];
      let kindle_smtp_config: Record<string, unknown>;
      try {
        const parsed = JSON.parse(pathRemappingsText || "[]");
        path_remappings = Array.isArray(parsed) ? parsed : [];
      } catch {
        throw new Error("Path remappings must be a JSON array.");
      }
      try {
        const parsed = JSON.parse(kindleSMTPText || "{}");
        kindle_smtp_config =
          parsed && typeof parsed === "object" && !Array.isArray(parsed)
            ? parsed
            : {};
      } catch {
        throw new Error("Kindle SMTP config must be a JSON object.");
      }
      return adminPatchBackend({
        ...draft,
        path_remappings,
        kindle_smtp_config,
      });
    },
    onSuccess: () => {
      toast.success("Settings saved");
      qc.invalidateQueries({ queryKey: ["admin", "backend"] });
      qc.invalidateQueries({ queryKey: ["admin", "cache"] });
    },
    onError: (e: Error) => toast.error(e.message),
  });

  if (loading) return <Skeleton className="h-72 w-full" />;

  return (
    <Card>
      <CardHeader className="border-b">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <CardTitle>Runtime settings</CardTitle>
            <CardDescription>
              Operational defaults for requests, streaming, cache, OPDS, and
              conversion.
            </CardDescription>
          </div>
          <Button
            type="button"
            onClick={() => save.mutate()}
            disabled={save.isPending}
          >
            <Save className="size-4" />
            Save settings
          </Button>
        </div>
      </CardHeader>
      <CardContent className="grid gap-4 pt-5 lg:grid-cols-2">
        <Field
          label="Auto-approve requests"
          description="When enabled, new requests are immediately submitted to the selected provider instead of waiting in the pending queue."
        >
          <label className="flex h-9 items-center gap-2 rounded-md border border-border px-3 text-sm">
            <input
              type="checkbox"
              checked={!!draft.auto_approve_requests}
              onChange={(e) =>
                setDraft((d) => ({
                  ...d,
                  auto_approve_requests: e.target.checked,
                }))
              }
            />
            Submit requests to the selected download provider without manual
            approval
          </label>
        </Field>
        <Field
          label="Default streaming mode"
          description="Proxy mode streams from the source on demand. Cache mode stores a local copy first, trading disk space for more predictable repeat reads."
        >
          <select
            value={draft.default_streaming_mode || "proxy"}
            onChange={(e) =>
              setDraft((d) => ({
                ...d,
                default_streaming_mode: e.target.value,
              }))
            }
            className="h-9 w-full rounded-md border border-border bg-background px-3 text-sm"
          >
            <option value="proxy">Proxy live from source</option>
            <option value="cache">Cache locally before serving</option>
          </select>
        </Field>
        <Field
          label="Cache max size (GB)"
          description="Upper bound for local cached ebook files. Eviction removes least-recently-used entries when the cache grows past this limit."
        >
          <Input
            type="number"
            min={1}
            value={draft.cache_max_size_gb ?? 10}
            onChange={(e) =>
              setDraft((d) => ({
                ...d,
                cache_max_size_gb: Number(e.target.value) || 1,
              }))
            }
          />
        </Field>
        <Field
          label="Cache download concurrency"
          description="Maximum parallel source downloads used while filling the local cache."
        >
          <Input
            type="number"
            min={1}
            value={draft.cache_download_concurrency ?? 4}
            onChange={(e) =>
              setDraft((d) => ({
                ...d,
                cache_download_concurrency: Number(e.target.value) || 1,
              }))
            }
          />
        </Field>
        <Field
          label="OPDS realm"
          description="Name shown by OPDS readers when they prompt for this catalog's credentials."
        >
          <Input
            value={draft.opds_realm || ""}
            onChange={(e) =>
              setDraft((d) => ({ ...d, opds_realm: e.target.value }))
            }
            placeholder="Continuum Library"
          />
        </Field>
        <Field
          label="Kepubify path"
          description="Optional path to kepubify for Kobo-optimized EPUB conversion before device delivery."
        >
          <Input
            value={draft.kepubify_path || ""}
            onChange={(e) =>
              setDraft((d) => ({ ...d, kepubify_path: e.target.value }))
            }
            placeholder="/usr/local/bin/kepubify"
          />
        </Field>
        <Field
          label="Standalone listen address"
          description="Optional direct HTTP listener for reverse-proxied OPDS, KOReader, Kobo, and Kindle routes. Restart the plugin after changing this value."
        >
          <Input
            value={draft.standalone_http_listen || ""}
            onChange={(e) =>
              setDraft((d) => ({
                ...d,
                standalone_http_listen: e.target.value,
              }))
            }
            placeholder="127.0.0.1:7878"
          />
        </Field>
        <Field
          label="Cache directory"
          description="Local directory used for cached ebook files. Restart the plugin after changing this path so cache workers reopen on the new directory."
        >
          <Input
            value={draft.cache_dir || ""}
            onChange={(e) =>
              setDraft((d) => ({ ...d, cache_dir: e.target.value }))
            }
            placeholder="/var/lib/continuum/ebooks-cache"
          />
        </Field>
        <Field
          label="Path remappings"
          description="JSON array used when source paths need to map into the portal container."
        >
          <textarea
            value={pathRemappingsText}
            onChange={(e) => setPathRemappingsText(e.target.value)}
            className="min-h-28 w-full rounded-md border border-border bg-background px-3 py-2 font-mono text-xs"
            spellCheck={false}
          />
        </Field>
        <Field
          label="Kindle SMTP config"
          description="JSON object for Kindle email delivery, including host, port, username, password, from, and tls."
        >
          <textarea
            value={kindleSMTPText}
            onChange={(e) => setKindleSMTPText(e.target.value)}
            className="min-h-28 w-full rounded-md border border-border bg-background px-3 py-2 font-mono text-xs"
            spellCheck={false}
          />
        </Field>
        <Field
          label="Default download provider"
          description="Provider used for new requests when the user does not choose one explicitly."
        >
          <div className="grid gap-2">
            {providers.length === 0 ? (
              <div className="rounded-md border border-border px-3 py-2 text-sm text-muted-foreground">
                No download providers are enabled.
              </div>
            ) : (
              providers.map((provider) => {
                const providerID = String(provider.id);
                const selected = draft.target_backend_plugin_id === providerID;
                return (
                  <button
                    key={provider.id}
                    type="button"
                    onClick={() =>
                      setDraft((d) => ({
                        ...d,
                        target_backend_plugin_id: providerID,
                      }))
                    }
                    className={[
                      "rounded-md border px-3 py-2 text-left text-sm transition-colors",
                      selected
                        ? "border-primary bg-primary/10 text-foreground"
                        : "border-border bg-background hover:bg-accent",
                    ].join(" ")}
                  >
                    <span className="block font-medium">
                      {provider.display_name}
                    </span>
                    <span className="block text-xs text-muted-foreground">
                      Install {provider.id} · {provider.plugin_id}
                    </span>
                    <span className="mt-1 block text-xs text-muted-foreground">
                      {providerProfile(provider).summary}
                    </span>
                  </button>
                );
              })
            )}
          </div>
        </Field>
      </CardContent>
    </Card>
  );
}

function Field({
  label,
  description,
  children,
}: {
  label: string;
  description?: string;
  children: React.ReactNode;
}) {
  return (
    <label className="block space-y-1.5">
      <span className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
        {label}
      </span>
      {description && (
        <span className="block text-xs leading-5 text-muted-foreground">
          {description}
        </span>
      )}
      {children}
    </label>
  );
}

function DataTable({
  loading,
  emptyIcon,
  emptyTitle,
  headers,
  rows,
}: {
  loading: boolean;
  emptyIcon: React.ReactNode;
  emptyTitle: string;
  headers: string[];
  rows: React.ReactNode[][];
}) {
  if (loading) return <Skeleton className="h-56 w-full" />;
  if (rows.length === 0) {
    return (
      <EmptyState
        icon={emptyIcon}
        title={emptyTitle}
        body="Nothing to show right now."
      />
    );
  }
  return (
    <Table>
      <TableHeader>
        <TableRow>
          {headers.map((header) => (
            <TableHead key={header}>{header}</TableHead>
          ))}
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.map((row, index) => (
          <TableRow key={index}>
            {row.map((cell, cellIndex) => (
              <TableCell key={cellIndex}>{cell}</TableCell>
            ))}
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}

function EmptyState({
  icon,
  title,
  body,
}: {
  icon: React.ReactNode;
  title: string;
  body: string;
}) {
  return (
    <div className="flex min-h-36 flex-col items-center justify-center rounded-lg border border-dashed border-border p-6 text-center">
      <div className="mb-3 text-muted-foreground">{icon}</div>
      <div className="font-medium">{title}</div>
      <p className="mt-1 max-w-md text-sm text-muted-foreground">{body}</p>
    </div>
  );
}

function StatusBadge({ status }: { status: string }) {
  const normalized = (status || "unknown").toLowerCase();
  const icon =
    normalized === "fulfilled" ||
    normalized === "sent" ||
    normalized === "ready" ||
    normalized === "active" ? (
      <CheckCircle2 className="size-3" />
    ) : normalized === "failed" ||
      normalized === "denied" ||
      normalized === "revoked" ? (
      <XCircle className="size-3" />
    ) : normalized === "pending" ||
      normalized === "queued" ||
      normalized === "submitted" ? (
      <Gauge className="size-3" />
    ) : (
      <AlertCircle className="size-3" />
    );
  const variant =
    normalized === "failed" ||
    normalized === "denied" ||
    normalized === "revoked"
      ? "destructive"
      : normalized === "pending" ||
          normalized === "queued" ||
          normalized === "submitted"
        ? "outline"
        : "secondary";
  return (
    <Badge variant={variant}>
      {icon}
      {titleCase(normalized)}
    </Badge>
  );
}

function text(row: unknown, ...keys: string[]): string {
  const value = field(row, ...keys);
  if (value == null) return "";
  if (value instanceof Date) return value.toISOString();
  return String(value);
}

function number(row: unknown, ...keys: string[]): number {
  const value = field(row, ...keys);
  if (typeof value === "number") return value;
  if (typeof value === "string") return Number(value) || 0;
  return 0;
}

function field(row: unknown, ...keys: string[]): unknown {
  if (!row || typeof row !== "object") return undefined;
  const record = row as Row;
  for (const key of keys) {
    if (key in record) return record[key];
  }
  return undefined;
}

function titleCase(value: string): string {
  return value
    .replace(/_/g, " ")
    .replace(/\w\S*/g, (word) => word.charAt(0).toUpperCase() + word.slice(1));
}

function formatDate(value?: string | null): string {
  if (!value || value === "0001-01-01T00:00:00Z") return "Never";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  }).format(date);
}

function formatBytes(bytes: number): string {
  if (!bytes) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit += 1;
  }
  return `${value >= 10 || unit === 0 ? value.toFixed(0) : value.toFixed(1)} ${units[unit]}`;
}
