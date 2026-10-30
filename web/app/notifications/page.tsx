"use client";

import { useMemo, useState } from "react";
import { useInfiniteQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Bell, CheckCheck } from "lucide-react";
import { AppShell } from "@/components/layout/AppShell";
import { Breadcrumbs } from "@/components/layout/Breadcrumbs";
import { NotificationItem } from "@/components/notifications/NotificationItem";
import { useSpaceKeys } from "@/components/notifications/useSpaceKeys";
import { listNotifications, markNotificationsRead, type Notification } from "@/lib/api";
import { Button } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";
import { Spinner } from "@/components/ui/Spinner";
import { Tabs, type TabItem } from "@/components/ui/Tabs";
import { useToast } from "@/components/ui/Toast";

const PAGE_SIZE = 30;
type Filter = "all" | "unread";

const TABS: TabItem<Filter>[] = [
  { id: "all", label: "All" },
  { id: "unread", label: "Unread" },
];

export default function NotificationsPage() {
  const qc = useQueryClient();
  const toast = useToast();
  const [filter, setFilter] = useState<Filter>("all");
  const spaceKeys = useSpaceKeys();

  const list = useInfiniteQuery({
    queryKey: ["notifications", "list", filter],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam }) =>
      listNotifications({ unread: filter === "unread", before: pageParam, limit: PAGE_SIZE }),
    getNextPageParam: (last) =>
      last.length >= PAGE_SIZE ? last[last.length - 1].created_at : undefined,
  });

  const items = useMemo(() => {
    const seen = new Set<string>();
    const out: Notification[] = [];
    for (const page of list.data?.pages ?? []) {
      for (const n of page) {
        if (seen.has(n.id)) continue;
        seen.add(n.id);
        out.push(n);
      }
    }
    return out;
  }, [list.data]);

  const hasUnread = items.some((n) => !n.read_at);

  const markRead = useMutation({
    mutationFn: (target: { ids: string[] } | { all: true }) => markNotificationsRead(target),
    onSettled: () => {
      qc.invalidateQueries({ queryKey: ["unread"] });
      qc.invalidateQueries({ queryKey: ["notifications"] });
    },
    onError: (e: Error) => toast.error(e.message),
  });

  return (
    <AppShell topSlot={<Breadcrumbs items={[{ label: "Notifications" }]} />}>
      <div className="mx-auto flex w-full max-w-3xl flex-col gap-4 p-4 sm:p-6">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h1 className="text-[20px] font-semibold tracking-tight text-ink-900">Notifications</h1>
          <Button
            variant="secondary"
            disabled={markRead.isPending || (!hasUnread && filter === "all")}
            onClick={() => markRead.mutate({ all: true })}
          >
            <CheckCheck size={14} /> Mark all read
          </Button>
        </div>

        <Tabs tabs={TABS} value={filter} onChange={setFilter} />

        <div className="surface overflow-hidden rounded-lg border border-ink-200">
          {list.isPending ? (
            <div className="flex justify-center py-16">
              <Spinner size={20} />
            </div>
          ) : list.isError ? (
            <p role="alert" className="p-6 text-sm text-danger-600">
              {list.error.message}
            </p>
          ) : items.length === 0 ? (
            <div className="p-6">
              <EmptyState
                icon={<Bell size={18} />}
                title={filter === "unread" ? "No unread notifications" : "No notifications yet"}
                description="Watch pages and spaces to get notified when they change."
              />
            </div>
          ) : (
            <ul className="divide-y divide-ink-100">
              {items.map((n) => (
                <li key={n.id}>
                  <NotificationItem
                    notification={n}
                    spaceKeyById={spaceKeys}
                    onOpen={(x) => {
                      if (!x.read_at) markRead.mutate({ ids: [x.id] });
                    }}
                  />
                </li>
              ))}
            </ul>
          )}
        </div>

        {list.hasNextPage && (
          <div className="flex justify-center">
            <Button
              variant="secondary"
              disabled={list.isFetchingNextPage}
              onClick={() => list.fetchNextPage()}
            >
              {list.isFetchingNextPage ? "Loading…" : "Load more"}
            </Button>
          </div>
        )}
      </div>
    </AppShell>
  );
}
