import { useMemo } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Bell } from "lucide-react";
import {
  getNotificationCatalog,
  listNotificationPrefs,
  putNotificationPref,
} from "@/lib/api";
import { Card } from "@/components/ui/card";
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
          Manage notification preferences and other personal settings.
        </p>
      </header>
      <NotificationPrefsCard />
    </div>
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
