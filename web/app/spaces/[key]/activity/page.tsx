"use client";

import { Fragment, useMemo } from "react";
import { useParams } from "next/navigation";
import { useInfiniteQuery } from "@tanstack/react-query";
import { Activity as ActivityIcon } from "lucide-react";
import { ActivityItem, activityDay } from "@/components/activity/ActivityItem";
import { getSpaceActivity, type Activity } from "@/lib/api";
import { Button } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";
import { Spinner } from "@/components/ui/Spinner";

const PAGE_SIZE = 30;

export default function SpaceActivityPage() {
  const params = useParams<{ key: string }>();
  const key = params.key;

  const feed = useInfiniteQuery({
    queryKey: ["activity", key],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam }) => getSpaceActivity(key, { before: pageParam, limit: PAGE_SIZE }),
    getNextPageParam: (last) =>
      last.length >= PAGE_SIZE ? last[last.length - 1].created_at : undefined,
  });

  const rows = useMemo(() => {
    const seen = new Set<string>();
    const out: Activity[] = [];
    for (const page of feed.data?.pages ?? []) {
      for (const a of page) {
        if (seen.has(a.id)) continue;
        seen.add(a.id);
        out.push(a);
      }
    }
    return out;
  }, [feed.data]);

  return (
    <div className="mx-auto flex w-full max-w-3xl flex-col gap-4 p-4 sm:p-6">
      <div>
        <h1 className="text-[20px] font-semibold tracking-tight text-ink-900">Activity</h1>
        <p className="mt-1 text-sm text-ink-600">Recent changes across {key}.</p>
      </div>

      {feed.isPending ? (
        <div className="flex justify-center py-16">
          <Spinner size={20} />
        </div>
      ) : feed.isError ? (
        <p role="alert" className="text-sm text-danger-600">
          {feed.error.message}
        </p>
      ) : rows.length === 0 ? (
        <EmptyState
          icon={<ActivityIcon size={18} />}
          title="No activity yet"
          description="Publishing, moving and commenting on pages will show up here."
        />
      ) : (
        <div className="surface overflow-hidden rounded-lg border border-ink-200">
          {rows.map((a, i) => {
            const day = activityDay(a.created_at);
            const showDay = i === 0 || day !== activityDay(rows[i - 1].created_at);
            return (
              <Fragment key={a.id}>
                {showDay && (
                  <div className="border-b border-ink-200 bg-ink-50 px-4 py-1.5 text-[11px] font-semibold uppercase tracking-wider text-ink-600">
                    {day}
                  </div>
                )}
                <div className="border-b border-ink-100 last:border-b-0">
                  <ActivityItem activity={a} spaceKey={key} />
                </div>
              </Fragment>
            );
          })}
        </div>
      )}

      {feed.hasNextPage && (
        <div className="flex justify-center">
          <Button
            variant="secondary"
            disabled={feed.isFetchingNextPage}
            onClick={() => feed.fetchNextPage()}
          >
            {feed.isFetchingNextPage ? "Loading…" : "Load more"}
          </Button>
        </div>
      )}
    </div>
  );
}
