import { useEffect, useState } from "react";
import { Link } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Flame, BookOpen, Target, Trophy } from "lucide-react";
import {
  getGoalProgress,
  getStreak,
  getYearStats,
  putGoal,
  type GoalProgress,
} from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";

// Ebook stats dashboard. Slimmer than the audiobook version — no
// session-time telemetry on the ebook side, so no heatmap or
// hours-listened headline. Books-finished + distinct active days
// + top books from year-in-review + goals progress + streak.

export default function Stats() {
  const year = new Date().getUTCFullYear();
  const streak = useQuery({ queryKey: ["streak"], queryFn: () => getStreak() });
  const goalProgress = useQuery({
    queryKey: ["goal-progress", year],
    queryFn: () => getGoalProgress(year),
  });
  const yearStats = useQuery({
    queryKey: ["year-stats", year],
    queryFn: () => getYearStats(year),
  });

  return (
    <div className="space-y-6">
      <header>
        <h2 className="text-2xl font-semibold tracking-tight">Your stats</h2>
        <p className="text-muted-foreground text-sm">
          Reading activity, streaks, and goal progress for {year}.
        </p>
      </header>

      <div className="grid gap-4 sm:grid-cols-3">
        <StatCard
          icon={<Flame className="size-5" />}
          label="Current streak"
          value={
            streak.isLoading ? (
              <Skeleton className="h-7 w-14" />
            ) : (
              <>{streak.data?.current ?? 0} days</>
            )
          }
          hint={
            streak.data?.longest
              ? `Longest: ${streak.data.longest} days`
              : "Open a book today to begin"
          }
        />
        <StatCard
          icon={<Trophy className="size-5" />}
          label="Books finished"
          value={
            yearStats.isLoading ? (
              <Skeleton className="h-7 w-14" />
            ) : (
              <>{yearStats.data?.books_finished ?? 0}</>
            )
          }
        />
        <StatCard
          icon={<Target className="size-5" />}
          label="Active days"
          value={
            yearStats.isLoading ? (
              <Skeleton className="h-7 w-14" />
            ) : (
              <>{yearStats.data?.distinct_days ?? 0}</>
            )
          }
        />
      </div>

      <GoalsCard
        year={year}
        progress={goalProgress.data?.goals ?? []}
        loading={goalProgress.isLoading}
      />

      <TopBooksCard
        top={yearStats.data?.top_books ?? []}
        loading={yearStats.isLoading}
      />
    </div>
  );
}

function StatCard({
  icon,
  label,
  value,
  hint,
}: {
  icon: React.ReactNode;
  label: string;
  value: React.ReactNode;
  hint?: string;
}) {
  return (
    <Card className="bg-surface p-4">
      <div className="text-muted-foreground flex items-center gap-2 text-xs">
        {icon}
        {label}
      </div>
      <div className="mt-2 text-2xl font-semibold tabular-nums">{value}</div>
      {hint && <div className="text-muted-foreground mt-1 text-xs">{hint}</div>}
    </Card>
  );
}

function GoalsCard({
  year,
  progress,
  loading,
}: {
  year: number;
  progress: GoalProgress[];
  loading: boolean;
}) {
  const qc = useQueryClient();
  const [booksTarget, setBooksTarget] = useState<number | "">("");

  useEffect(() => {
    const books = progress.find((g) => g.kind === "books")?.target;
    setBooksTarget(books ?? "");
  }, [year, progress.length]);

  const save = useMutation({
    mutationFn: async () => {
      if (booksTarget && Number(booksTarget) > 0) {
        await putGoal(year, "books", Number(booksTarget));
      }
    },
    onSuccess: () => {
      toast.success("Goal saved");
      qc.invalidateQueries({ queryKey: ["goal-progress", year] });
    },
    onError: (err) => toast.error(`Save failed: ${err}`),
  });

  return (
    <Card className="bg-surface p-4">
      <div className="mb-4">
        <h3 className="font-medium">Yearly goal</h3>
        <p className="text-muted-foreground text-xs">
          Set how many books you'd like to finish in {year}.
        </p>
      </div>

      {loading ? (
        <Skeleton className="h-20 w-full" />
      ) : progress.length === 0 ? (
        <p className="text-muted-foreground mb-4 text-sm">No goal set yet.</p>
      ) : (
        <div className="mb-4 space-y-4">
          {progress.map((g) => (
            <GoalRow key={g.kind} goal={g} />
          ))}
        </div>
      )}

      <div className="bg-border my-4 h-px" />
      <div className="grid gap-3 sm:grid-cols-[1fr,auto]">
        <div>
          <Label htmlFor="books-target">Books target ({year})</Label>
          <Input
            id="books-target"
            type="number"
            min={0}
            placeholder="e.g. 24"
            value={booksTarget}
            onChange={(e) =>
              setBooksTarget(e.target.value ? Number(e.target.value) : "")
            }
          />
        </div>
        <Button
          className="self-end"
          onClick={() => save.mutate()}
          disabled={save.isPending}
        >
          Save
        </Button>
      </div>
    </Card>
  );
}

function GoalRow({ goal }: { goal: GoalProgress }) {
  const pct = Math.max(0, Math.min(100, goal.percent_complete));
  return (
    <div>
      <div className="mb-1 flex items-baseline justify-between text-sm">
        <span className="font-medium capitalize">{goal.kind}</span>
        <span className="text-muted-foreground tabular-nums text-xs">
          {goal.actual} / {goal.target}
          {goal.on_pace_for_target ? " · on pace" : " · behind pace"}
        </span>
      </div>
      <div className="bg-muted h-2 overflow-hidden rounded-full">
        <div
          className={`h-full ${goal.on_pace_for_target ? "bg-primary" : "bg-amber-500"}`}
          style={{ width: `${pct}%` }}
        />
      </div>
    </div>
  );
}

function TopBooksCard({
  top,
  loading,
}: {
  top: { book_id: string; title?: string; authors?: string[]; progress?: number; last_read_at?: string }[];
  loading: boolean;
}) {
  return (
    <Card className="bg-surface p-4">
      <h3 className="mb-3 flex items-center gap-2 font-medium">
        <BookOpen className="size-4" />
        Most read this year
      </h3>
      {loading ? (
        <Skeleton className="h-32 w-full" />
      ) : top.length === 0 ? (
        <p className="text-muted-foreground text-sm">No reading activity yet.</p>
      ) : (
        <ol className="space-y-2">
          {top.slice(0, 10).map((b, i) => (
            <li
              key={b.book_id}
              className="flex items-baseline justify-between text-sm"
            >
              <span className="min-w-0 flex-1 truncate">
                <span className="text-muted-foreground mr-2 tabular-nums">
                  {i + 1}.
                </span>
                <Link
                  to={`/books/${encodeURIComponent(b.book_id)}`}
                  className="font-medium hover:underline"
                >
                  {b.title ?? b.book_id}
                </Link>
                {b.authors?.length ? (
                  <span className="text-muted-foreground ml-2">
                    by {b.authors.join(", ")}
                  </span>
                ) : null}
              </span>
              {typeof b.progress === "number" && (
                <span className="text-muted-foreground tabular-nums text-xs">
                  {Math.round((b.progress ?? 0) * 100)}%
                </span>
              )}
            </li>
          ))}
        </ol>
      )}
    </Card>
  );
}
