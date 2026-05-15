import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router";
import { fetchDownloadProviders, getMyRequest, type Request } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";

const steps = [
  "pending",
  "submitted",
  "acknowledged",
  "downloading",
  "fulfilled",
];

function statusLabel(status: string) {
  return status.replace(/_/g, " ").replace(/\b\w/g, (c) => c.toUpperCase());
}

function providerName(
  providers: Awaited<ReturnType<typeof fetchDownloadProviders>>,
  id: string,
) {
  const provider = providers.find((item) => String(item.id) === id);
  return provider ? provider.display_name : `Install ${id}`;
}

export default function RequestDetail() {
  const { id = "" } = useParams();
  const request = useQuery({
    queryKey: ["my-request", id],
    queryFn: () => getMyRequest(id),
    enabled: !!id,
  });
  const providers = useQuery({
    queryKey: ["download-providers"],
    queryFn: fetchDownloadProviders,
  });

  if (request.isLoading) return <Skeleton className="h-80 w-full" />;
  if (request.error || !request.data) {
    return (
      <div className="space-y-3">
        <p className="text-sm text-destructive">
          {(request.error as Error)?.message || "Request not found"}
        </p>
        <Button asChild variant="outline" size="sm">
          <Link to="/me/requests">Back to requests</Link>
        </Button>
      </div>
    );
  }

  const item = request.data;
  const terminal = ["fulfilled", "failed", "denied"].includes(item.status);

  return (
    <div className="space-y-5">
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            {item.title}
          </h1>
          <p className="mt-1 text-sm text-muted-foreground">
            {(item.authors ?? []).join(", ") || item.isbn || "Reader request"}
          </p>
        </div>
        <Button asChild variant="outline" size="sm">
          <Link to="/me/requests">Back</Link>
        </Button>
      </header>

      <div className="grid gap-3 md:grid-cols-3">
        <InfoCard title="Status" value={statusLabel(item.status)} />
        <InfoCard
          title="Download provider"
          value={providerName(providers.data ?? [], item.target_plugin_id)}
        />
        <InfoCard
          title="Format"
          value={(item.format_pref || "epub").toUpperCase()}
        />
      </div>

      <section className="rounded-lg border border-border bg-card p-4">
        <h2 className="mb-3 text-sm font-semibold">Timeline</h2>
        <div className="space-y-3">
          {buildTimeline(item).map((event) => (
            <div key={event.label} className="flex gap-3">
              <div
                className={[
                  "mt-1 size-2 rounded-full",
                  event.active ? "bg-primary" : "bg-muted-foreground/30",
                ].join(" ")}
              />
              <div>
                <div className="text-sm font-medium">{event.label}</div>
                <div className="text-xs text-muted-foreground">
                  {event.detail}
                </div>
              </div>
            </div>
          ))}
        </div>
      </section>

      {(item.external_id ||
        item.source_id ||
        item.failure_reason ||
        item.denied_reason) && (
        <section className="rounded-lg border border-border bg-card p-4">
          <h2 className="mb-3 text-sm font-semibold">Provider details</h2>
          <dl className="grid gap-3 text-sm md:grid-cols-2">
            {item.external_id && (
              <Detail label="External job" value={item.external_id} />
            )}
            {item.source_id && (
              <Detail label="Source ID" value={item.source_id} />
            )}
            {item.failure_reason && (
              <Detail label="Failure" value={item.failure_reason} />
            )}
            {item.denied_reason && (
              <Detail label="Denied" value={item.denied_reason} />
            )}
          </dl>
        </section>
      )}

      {item.fulfilled_book_id && (
        <Button asChild>
          <Link to={`/${encodeURIComponent(item.fulfilled_book_id)}`}>
            Open fulfilled book
          </Link>
        </Button>
      )}
      {!terminal && (
        <p className="text-sm text-muted-foreground">
          This request is still active. Status updates appear here after the
          provider reports progress.
        </p>
      )}
    </div>
  );
}

function InfoCard({ title, value }: { title: string; value: string }) {
  return (
    <div className="rounded-lg border border-border bg-card p-4">
      <div className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
        {title}
      </div>
      <div className="mt-2 text-lg font-semibold">{value}</div>
    </div>
  );
}

function Detail({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
        {label}
      </dt>
      <dd className="mt-1 break-all">{value}</dd>
    </div>
  );
}

function buildTimeline(item: Request) {
  const statusIndex = Math.max(steps.indexOf(item.status), 0);
  const base = steps.map((step, index) => ({
    label: statusLabel(step),
    active: index <= statusIndex && !["failed", "denied"].includes(item.status),
    detail:
      index === 0
        ? `Created ${formatDate(item.created_at)}`
        : index <= statusIndex
          ? `Updated ${formatDate(item.updated_at)}`
          : "Waiting",
  }));
  if (item.status === "failed") {
    base.push({
      label: "Failed",
      active: true,
      detail: item.failure_reason || `Updated ${formatDate(item.updated_at)}`,
    });
  }
  if (item.status === "denied") {
    base.push({
      label: "Denied",
      active: true,
      detail: item.denied_reason || `Updated ${formatDate(item.updated_at)}`,
    });
  }
  return base;
}

function formatDate(value?: string) {
  if (!value) return "Never";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  }).format(date);
}
