import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { toast } from "sonner";
import {
  deleteKosync,
  getKosyncStatus,
  registerKosync,
} from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";

// Apps page: per-spec Layer 5.1, this is the user's settings panel for the
// reader integrations (KOReader kosync). Each section is
// self-contained — invalidate only the relevant query on mutation.
export default function Apps() {
  return (
    <div className="space-y-8">
      <h1 className="text-xl font-semibold">Apps & integrations</h1>
      <KOReaderSection />
    </div>
  );
}

function KOReaderSection() {
  const qc = useQueryClient();
  const status = useQuery({ queryKey: ["kosync"], queryFn: getKosyncStatus });
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const reg = useMutation({
    mutationFn: () => registerKosync(username, password),
    onSuccess: () => {
      toast.success("KOReader sync configured");
      setPassword("");
      qc.invalidateQueries({ queryKey: ["kosync"] });
    },
    onError: (e: Error) => toast.error(e.message),
  });
  const del = useMutation({
    mutationFn: deleteKosync,
    onSuccess: () => qc.invalidateQueries({ queryKey: ["kosync"] }),
    onError: (e: Error) => toast.error(e.message),
  });
  return (
    <section>
      <h2 className="mb-2 text-base font-semibold">KOReader sync</h2>
      {status.isLoading ? (
        <Skeleton className="h-16 w-full" />
      ) : status.data?.registered ? (
        <div className="flex items-center justify-between rounded-lg border border-border bg-card px-4 py-2 text-sm">
          <span>
            Registered as <code>{status.data.kosync_username}</code>
          </span>
          <Button
            size="sm"
            variant="ghost"
            onClick={() => del.mutate()}
            disabled={del.isPending}
          >
            Disconnect
          </Button>
        </div>
      ) : (
        <form
          onSubmit={(e) => {
            e.preventDefault();
            reg.mutate();
          }}
          className="flex flex-wrap gap-2"
        >
          <input
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            placeholder="kosync username"
            className="flex-1 rounded-md border border-border bg-background px-2 py-1.5 text-sm"
          />
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            placeholder="kosync password"
            className="flex-1 rounded-md border border-border bg-background px-2 py-1.5 text-sm"
          />
          <Button
            type="submit"
            disabled={!username || !password || reg.isPending}
          >
            Register
          </Button>
        </form>
      )}
    </section>
  );
}
