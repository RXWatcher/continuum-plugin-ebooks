import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router";
import { toast } from "sonner";
import { Trash2 } from "lucide-react";
import { cancelRequest, listMyRequests } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";

const STATUS_LABELS: Record<string, string> = {
  pending: "Pending approval",
  submitted: "Submitted",
  acknowledged: "Acknowledged",
  searching: "Searching",
  found: "Found",
  downloading: "Downloading",
  fulfilled: "Fulfilled",
  failed: "Failed",
  denied: "Denied",
};

export default function MyRequests() {
  const qc = useQueryClient();
  const q = useQuery({ queryKey: ["my-requests"], queryFn: listMyRequests });
  const cancel = useMutation({
    mutationFn: (id: string) => cancelRequest(id),
    onSuccess: () => {
      toast.success("Request cancelled");
      qc.invalidateQueries({ queryKey: ["my-requests"] });
    },
    onError: (e: Error) => toast.error(e.message),
  });

  return (
    <div>
      <div className="mb-4 flex items-baseline justify-between">
        <h1 className="text-xl font-semibold">My requests</h1>
        <Button asChild size="sm">
          <Link to="/me/submit">+ New request</Link>
        </Button>
      </div>
      {q.isLoading ? (
        <div className="space-y-2">
          {Array.from({ length: 5 }).map((_, i) => (
            <Skeleton key={i} className="h-16 w-full" />
          ))}
        </div>
      ) : q.error ? (
        <p className="text-sm text-destructive">{(q.error as Error).message}</p>
      ) : q.data && q.data.items.length > 0 ? (
        <ul className="space-y-2">
          {q.data.items.map((r) => (
            <li
              key={r.id}
              className="flex items-center justify-between gap-3 rounded-lg border border-border bg-card p-3"
            >
              <Link
                to={`/me/requests/${encodeURIComponent(r.id)}`}
                className="min-w-0 flex-1"
              >
                <p className="truncate text-sm font-medium hover:text-primary">
                  {r.title}
                </p>
                {r.authors && r.authors.length > 0 && (
                  <p className="truncate text-xs text-muted-foreground">
                    {r.authors.join(", ")}
                  </p>
                )}
                <p className="mt-1 text-xs text-muted-foreground">
                  {STATUS_LABELS[r.status] ?? r.status}
                  {r.denied_reason ? ` — ${r.denied_reason}` : ""}
                  {r.failure_reason ? ` — ${r.failure_reason}` : ""}
                </p>
              </Link>
              {["pending", "submitted"].includes(r.status) && (
                <Button
                  size="icon"
                  variant="ghost"
                  onClick={() => cancel.mutate(r.id)}
                  disabled={cancel.isPending}
                  aria-label="Cancel"
                >
                  <Trash2 className="size-4" />
                </Button>
              )}
            </li>
          ))}
        </ul>
      ) : (
        <p className="text-sm text-muted-foreground">No requests yet.</p>
      )}
    </div>
  );
}
