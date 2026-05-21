import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  Bell,
  CheckCircle2,
  Copy,
  Link2,
  Mail,
  Plus,
  Smartphone,
  Trash2,
  XCircle,
} from "lucide-react";
import {
  checkHardcoverAuth,
  checkReadwiseAuth,
  createEreaderDevice,
  deleteEreaderDevice,
  deleteHardcoverToken,
  deleteReadwiseToken,
  deleteShareLink,
  getHardcoverToken,
  getNotificationCatalog,
  getReadwiseToken,
  listEreaderDevices,
  listNotificationPrefs,
  listShareLinks,
  putHardcoverToken,
  putNotificationPref,
  putReadwiseToken,
  type EreaderDevice,
  type ShareLink,
} from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";

const CATEGORY_LABELS: Record<string, string> = {
  new_book: "A new ebook lands in a library you can see",
  reading_reminder: "Reading-streak reminder when you're falling behind",
  request_fulfilled: "An ebook you requested arrives",
  backup_complete: "Admin: backup job completes",
  share_used: "Someone opens a share link you created",
};

const DELIVERY_LABELS: Record<string, string> = {
  inapp: "In-app",
  email: "Email",
  push: "Push",
};

export default function Settings() {
  return (
    <div className="space-y-6">
      <header>
        <h2 className="text-2xl font-semibold tracking-tight">Settings</h2>
        <p className="text-muted-foreground text-sm">
          Manage your devices, integrations, and notification preferences.
        </p>
      </header>
      <EreaderDevicesCard />
      <ReadwiseCard />
      <HardcoverCard />
      <ShareLinksCard />
      <NotificationPrefsCard />
    </div>
  );
}

function ShareLinksCard() {
  const qc = useQueryClient();
  const links = useQuery({
    queryKey: ["share-links"],
    queryFn: listShareLinks,
  });
  const remove = useMutation({
    mutationFn: (id: string) => deleteShareLink(id),
    onSuccess: () => {
      toast.success("Share link revoked");
      qc.invalidateQueries({ queryKey: ["share-links"] });
    },
    onError: (err) => toast.error(`Revoke failed: ${err}`),
  });

  return (
    <Card className="bg-surface p-4">
      <div className="mb-4 flex items-center gap-2">
        <Link2 className="size-5" />
        <h3 className="font-medium">Share links</h3>
      </div>
      <p className="text-muted-foreground mb-3 text-xs">
        Anyone with one of these links can open the linked book until it
        expires or hits the use cap. Revoking instantly disables a link.
      </p>

      {links.isLoading ? (
        <Skeleton className="h-20 w-full" />
      ) : (links.data?.items ?? []).length === 0 ? (
        <p className="text-muted-foreground text-sm">No share links yet.</p>
      ) : (
        <ul className="space-y-2">
          {links.data!.items.map((l) => (
            <ShareLinkRow
              key={l.id}
              link={l}
              onDelete={() => remove.mutate(l.id)}
            />
          ))}
        </ul>
      )}
    </Card>
  );
}

function ShareLinkRow({
  link,
  onDelete,
}: {
  link: ShareLink;
  onDelete: () => void;
}) {
  const url = `${window.location.origin}/share/${link.slug}`;
  const expiresInDays = link.expires_at
    ? Math.max(
        0,
        Math.ceil(
          (new Date(link.expires_at).getTime() - Date.now()) /
            (1000 * 60 * 60 * 24),
        ),
      )
    : null;
  const usesRemaining =
    link.max_uses > 0 ? Math.max(0, link.max_uses - link.use_count) : null;
  return (
    <li className="bg-background flex items-center justify-between gap-2 rounded-md border border-dashed p-3 text-sm">
      <div className="min-w-0 flex-1">
        <div className="truncate font-medium">{link.item_id}</div>
        <div className="text-muted-foreground text-xs">
          {expiresInDays !== null
            ? `Expires in ${expiresInDays} day${expiresInDays === 1 ? "" : "s"}`
            : "No expiry"}
          {" · "}
          {usesRemaining !== null
            ? `${usesRemaining} use${usesRemaining === 1 ? "" : "s"} left`
            : `${link.use_count} opens`}
        </div>
      </div>
      <Button
        size="icon"
        variant="ghost"
        title="Copy link"
        onClick={() => {
          navigator.clipboard.writeText(url).then(
            () => toast.success("Link copied"),
            () => toast.error("Copy failed"),
          );
        }}
      >
        <Copy className="size-4" />
      </Button>
      <Button size="icon" variant="ghost" title="Revoke" onClick={onDelete}>
        <Trash2 className="size-4" />
      </Button>
    </li>
  );
}

function EreaderDevicesCard() {
  const qc = useQueryClient();
  const devices = useQuery({
    queryKey: ["ereader-devices"],
    queryFn: listEreaderDevices,
  });
  const [draftName, setDraftName] = useState("");
  const [draftEmail, setDraftEmail] = useState("");
  const [draftVendor, setDraftVendor] = useState("kindle");
  const [draftFormat, setDraftFormat] = useState("epub");

  const create = useMutation({
    mutationFn: () =>
      createEreaderDevice({
        name: draftName.trim(),
        email: draftEmail.trim(),
        vendor: draftVendor,
        preferred_format: draftFormat,
      }),
    onSuccess: () => {
      setDraftName("");
      setDraftEmail("");
      qc.invalidateQueries({ queryKey: ["ereader-devices"] });
    },
    onError: (err) => toast.error(`Add device failed: ${err}`),
  });

  const remove = useMutation({
    mutationFn: (id: string) => deleteEreaderDevice(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["ereader-devices"] }),
  });

  return (
    <Card className="bg-surface p-4">
      <div className="mb-4 flex items-center gap-2">
        <Smartphone className="size-5" />
        <h3 className="font-medium">E-reader devices</h3>
      </div>
      <p className="text-muted-foreground mb-4 text-xs">
        Add the email address your Kindle, Kobo, or Boox accepts. From any
        book page, &quot;Send to device&quot; will mail the file there.
      </p>

      {devices.isLoading ? (
        <Skeleton className="h-24 w-full" />
      ) : (devices.data?.items ?? []).length === 0 ? (
        <p className="text-muted-foreground mb-4 text-sm">No devices yet.</p>
      ) : (
        <ul className="mb-4 space-y-2">
          {devices.data!.items.map((d) => (
            <DeviceRow key={d.id} device={d} onDelete={() => remove.mutate(d.id)} />
          ))}
        </ul>
      )}

      <div className="bg-border my-4 h-px" />

      <div className="grid gap-3 sm:grid-cols-2">
        <div>
          <Label htmlFor="device-name">Name</Label>
          <Input
            id="device-name"
            placeholder="My Kindle"
            value={draftName}
            onChange={(e) => setDraftName(e.target.value)}
          />
        </div>
        <div>
          <Label htmlFor="device-email">Send-to email</Label>
          <Input
            id="device-email"
            placeholder="me@kindle.com"
            value={draftEmail}
            onChange={(e) => setDraftEmail(e.target.value)}
          />
        </div>
        <div>
          <Label htmlFor="device-vendor">Vendor</Label>
          <Select value={draftVendor} onValueChange={setDraftVendor}>
            <SelectTrigger id="device-vendor">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="kindle">Kindle</SelectItem>
              <SelectItem value="kobo">Kobo</SelectItem>
              <SelectItem value="boox">Boox</SelectItem>
              <SelectItem value="generic">Other</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div>
          <Label htmlFor="device-format">Preferred format</Label>
          <Select value={draftFormat} onValueChange={setDraftFormat}>
            <SelectTrigger id="device-format">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="epub">EPUB</SelectItem>
              <SelectItem value="mobi">MOBI</SelectItem>
              <SelectItem value="azw3">AZW3</SelectItem>
              <SelectItem value="pdf">PDF</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </div>
      <Button
        className="mt-3"
        onClick={() => create.mutate()}
        disabled={!draftName.trim() || !draftEmail.trim() || create.isPending}
      >
        <Plus className="size-4" />
        <span className="ml-1">Add device</span>
      </Button>
    </Card>
  );
}

function DeviceRow({
  device,
  onDelete,
}: {
  device: EreaderDevice;
  onDelete: () => void;
}) {
  return (
    <li className="bg-background flex items-center justify-between rounded-md border border-dashed p-3 text-sm">
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2 font-medium">
          <Mail className="text-muted-foreground size-4" />
          {device.name}
        </div>
        <div className="text-muted-foreground truncate text-xs">
          {device.email} · {device.vendor ?? "device"} ·{" "}
          {device.preferred_format ?? "epub"}
        </div>
      </div>
      <Button size="icon" variant="ghost" onClick={onDelete} title="Delete device">
        <Trash2 className="size-4" />
      </Button>
    </li>
  );
}

function ReadwiseCard() {
  return (
    <TokenIntegrationCard
      title="Readwise"
      description="Push highlights + notes to your Readwise account. Token never leaves the server."
      docsHref="https://readwise.io/access_token"
      tokenKey="readwise"
      getToken={getReadwiseToken}
      putToken={putReadwiseToken}
      deleteToken={deleteReadwiseToken}
      checkAuth={checkReadwiseAuth}
    />
  );
}

function HardcoverCard() {
  return (
    <TokenIntegrationCard
      title="Hardcover"
      description="Sync your reading status (want / current / read) to Hardcover.app."
      docsHref="https://hardcover.app/account/api"
      tokenKey="hardcover"
      getToken={getHardcoverToken}
      putToken={putHardcoverToken}
      deleteToken={deleteHardcoverToken}
      checkAuth={checkHardcoverAuth}
    />
  );
}

// TokenIntegrationCard is the shared shape for any third-party API
// key holder. The card shows the masked token + a paste-new-token
// input + a "Verify" button. Server-side stored unmasked; client
// only ever sees the masked form on read.
function TokenIntegrationCard({
  title,
  description,
  docsHref,
  tokenKey,
  getToken,
  putToken,
  deleteToken,
  checkAuth,
}: {
  title: string;
  description: string;
  docsHref: string;
  tokenKey: string;
  getToken: () => Promise<{ token: string | null }>;
  putToken: (token: string) => Promise<unknown>;
  deleteToken: () => Promise<unknown>;
  checkAuth: () => Promise<{ ok: boolean; username?: string; error?: string }>;
}) {
  const qc = useQueryClient();
  const token = useQuery({
    queryKey: [`${tokenKey}-token`],
    queryFn: getToken,
  });
  const [draft, setDraft] = useState("");
  const save = useMutation({
    mutationFn: () => putToken(draft.trim()),
    onSuccess: () => {
      setDraft("");
      toast.success(`${title} token saved`);
      qc.invalidateQueries({ queryKey: [`${tokenKey}-token`] });
    },
    onError: (err) => toast.error(`Save failed: ${err}`),
  });
  const remove = useMutation({
    mutationFn: () => deleteToken(),
    onSuccess: () => {
      toast.success(`${title} token removed`);
      qc.invalidateQueries({ queryKey: [`${tokenKey}-token`] });
    },
  });
  const verify = useMutation({
    mutationFn: () => checkAuth(),
    onSuccess: (res) => {
      if (res.ok) {
        toast.success(`${title}: signed in${res.username ? ` as ${res.username}` : ""}`);
      } else {
        toast.error(`${title}: ${res.error ?? "auth check failed"}`);
      }
    },
    onError: (err) => toast.error(`Verify failed: ${err}`),
  });

  const masked = token.data?.token;
  const connected = !!masked;

  return (
    <Card className="bg-surface p-4">
      <div className="mb-2 flex items-center gap-2">
        {connected ? (
          <CheckCircle2 className="text-emerald-500 size-5" />
        ) : (
          <XCircle className="text-muted-foreground size-5" />
        )}
        <h3 className="font-medium">{title}</h3>
      </div>
      <p className="text-muted-foreground mb-3 text-xs">
        {description}{" "}
        <a className="underline" href={docsHref} target="_blank" rel="noreferrer">
          Where to find your token →
        </a>
      </p>

      {token.isLoading ? (
        <Skeleton className="h-10 w-full" />
      ) : connected ? (
        <div className="mb-3 flex items-center gap-2 text-sm">
          <span className="text-muted-foreground">Current:</span>
          <code className="bg-background rounded px-2 py-1 font-mono text-xs">
            {masked}
          </code>
          <Button size="sm" variant="ghost" onClick={() => verify.mutate()}>
            Verify
          </Button>
          <Button size="sm" variant="ghost" onClick={() => remove.mutate()}>
            <Trash2 className="size-4" />
          </Button>
        </div>
      ) : (
        <p className="text-muted-foreground mb-3 text-sm">Not connected.</p>
      )}

      <div className="flex gap-2">
        <Input
          type="password"
          placeholder={`Paste ${title} token`}
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
        />
        <Button onClick={() => save.mutate()} disabled={!draft.trim() || save.isPending}>
          Save
        </Button>
      </div>
    </Card>
  );
}

function NotificationPrefsCard() {
  const qc = useQueryClient();
  const catalog = useQuery({
    queryKey: ["notification-catalog"],
    queryFn: getNotificationCatalog,
  });
  const prefs = useQuery({
    queryKey: ["notification-prefs"],
    queryFn: listNotificationPrefs,
  });

  const setPref = useMutation({
    mutationFn: (vars: {
      category: string;
      delivery: string;
      enabled: boolean;
    }) => putNotificationPref(vars.category, vars.delivery, vars.enabled),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["notification-prefs"] }),
    onError: (err) => toast.error(`Update failed: ${err}`),
  });

  const enabledMap = useMemo(() => {
    const m = new Map<string, boolean>();
    for (const p of prefs.data?.items ?? []) {
      m.set(`${p.category}/${p.delivery}`, p.enabled);
    }
    return m;
  }, [prefs.data]);

  const loading = catalog.isLoading || prefs.isLoading;

  return (
    <Card className="bg-surface p-4">
      <div className="mb-4 flex items-center gap-2">
        <Bell className="size-5" />
        <h3 className="font-medium">Notifications</h3>
      </div>

      {loading ? (
        <Skeleton className="h-32 w-full" />
      ) : (
        <div className="space-y-3">
          {(catalog.data?.categories ?? []).map((category) => (
            <CategoryRow
              key={category}
              category={category}
              deliveries={catalog.data?.deliveries ?? []}
              isEnabled={(delivery) =>
                enabledMap.get(`${category}/${delivery}`) ?? true
              }
              onToggle={(delivery, enabled) =>
                setPref.mutate({ category, delivery, enabled })
              }
            />
          ))}
          {(catalog.data?.categories ?? []).length === 0 && (
            <p className="text-muted-foreground text-sm">
              No notification categories configured.
            </p>
          )}
        </div>
      )}
    </Card>
  );
}

function CategoryRow({
  category,
  deliveries,
  isEnabled,
  onToggle,
}: {
  category: string;
  deliveries: string[];
  isEnabled: (delivery: string) => boolean;
  onToggle: (delivery: string, enabled: boolean) => void;
}) {
  return (
    <div className="bg-background flex flex-wrap items-center justify-between gap-3 rounded-md border border-dashed p-3">
      <div className="min-w-0 flex-1">
        <div className="font-medium text-sm">
          {CATEGORY_LABELS[category] ?? category}
        </div>
        <div className="text-muted-foreground text-xs">{category}</div>
      </div>
      <div className="flex gap-4">
        {deliveries.map((delivery) => (
          <label key={delivery} className="flex items-center gap-2">
            <Switch
              checked={isEnabled(delivery)}
              onCheckedChange={(v) => onToggle(delivery, v)}
            />
            <span className="text-xs">
              {DELIVERY_LABELS[delivery] ?? delivery}
            </span>
          </label>
        ))}
      </div>
    </div>
  );
}
