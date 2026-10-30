"use client";

import Link from "next/link";
import { useCallback, useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Bell, CheckCheck } from "lucide-react";
import {
  listNotifications,
  markNotificationsRead,
  type Notification,
} from "@/lib/api";
import { Button } from "@/components/ui/Button";
import { Spinner } from "@/components/ui/Spinner";
import { useToast } from "@/components/ui/Toast";
import { NotificationItem } from "./NotificationItem";
import { UNREAD_KEY, useNotifications } from "./NotificationsProvider";
import { useSpaceKeys } from "./useSpaceKeys";

const LATEST_LIMIT = 8;
const LATEST_KEY = ["notifications", "latest"] as const;

export function NotificationBell() {
  const qc = useQueryClient();
  const toast = useToast();
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const spaceKeys = useSpaceKeys(open);

  const { unread: count } = useNotifications();

  const latest = useQuery({
    queryKey: LATEST_KEY,
    queryFn: () => listNotifications({ limit: LATEST_LIMIT }),
    enabled: open,
    staleTime: 0,
  });

  const markRead = useMutation({
    mutationFn: (target: { ids: string[] } | { all: true }) => markNotificationsRead(target),
    onSettled: () => {
      qc.invalidateQueries({ queryKey: UNREAD_KEY });
      qc.invalidateQueries({ queryKey: ["notifications"] });
    },
    onError: (e: Error) => toast.error(e.message),
  });

  const markAll = () => {
    qc.setQueryData(UNREAD_KEY, 0);
    markRead.mutate({ all: true });
  };

  const onOpenItem = useCallback(
    (n: Notification) => {
      setOpen(false);
      if (!n.read_at) {
        qc.setQueryData<number>(UNREAD_KEY, (c) => Math.max(0, (c ?? 1) - 1));
        markRead.mutate({ ids: [n.id] });
      }
    },
    [markRead, qc]
  );

  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      if (!rootRef.current?.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        setOpen(false);
        triggerRef.current?.focus();
      }
    };
    document.addEventListener("mousedown", onDown);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDown);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);

  const items = (latest.data ?? []).slice(0, LATEST_LIMIT);
  const badge = count > 99 ? "99+" : String(count);

  return (
    <div ref={rootRef} className="relative">
      <button
        ref={triggerRef}
        type="button"
        aria-haspopup="dialog"
        aria-expanded={open}
        aria-label={count > 0 ? `Notifications, ${count} unread` : "Notifications"}
        title="Notifications"
        onClick={() => setOpen((o) => !o)}
        className="relative flex h-9 w-9 shrink-0 items-center justify-center rounded-md text-ink-700 outline-none hover:bg-ink-100 hover:text-ink-900 focus-visible:ring-2 focus-visible:ring-brand-400"
      >
        <Bell size={16} />
        {count > 0 && (
          <span className="absolute right-0.5 top-0.5 flex h-4 min-w-[16px] items-center justify-center rounded-full bg-danger-600 px-1 text-[10px] font-semibold leading-none text-white">
            {badge}
          </span>
        )}
      </button>

      {open && (
        <div
          role="dialog"
          aria-label="Notifications"
          className="fixed inset-x-2 top-14 z-50 flex max-h-[min(34rem,calc(100vh-4.5rem))] flex-col overflow-hidden rounded-lg border border-ink-200 bg-ink-0 shadow-overlay animate-scale-in sm:absolute sm:inset-x-auto sm:right-0 sm:top-full sm:mt-1 sm:w-[26rem]"
        >
          <div className="flex shrink-0 items-center justify-between gap-2 border-b border-ink-200 px-3 py-2">
            <h2 className="text-sm font-semibold text-ink-900">Notifications</h2>
            <Button
              size="sm"
              variant="ghost"
              disabled={count === 0 || markRead.isPending}
              onClick={markAll}
            >
              <CheckCheck size={14} /> Mark all read
            </Button>
          </div>

          <div className="min-h-0 flex-1 divide-y divide-ink-100 overflow-y-auto">
            {latest.isPending ? (
              <div className="flex justify-center py-10">
                <Spinner size={18} />
              </div>
            ) : latest.isError ? (
              <p role="alert" className="px-4 py-8 text-center text-sm text-danger-600">
                {latest.error.message}
              </p>
            ) : items.length === 0 ? (
              <p className="px-4 py-10 text-center text-sm text-ink-600">
                You&rsquo;re all caught up.
              </p>
            ) : (
              items.map((n) => (
                <NotificationItem
                  key={n.id}
                  notification={n}
                  spaceKeyById={spaceKeys}
                  onOpen={onOpenItem}
                  compact
                />
              ))
            )}
          </div>

          <Link
            href="/notifications"
            onClick={() => setOpen(false)}
            className="shrink-0 border-t border-ink-200 px-3 py-2.5 text-center text-sm font-medium text-brand-700 hover:bg-ink-50"
          >
            View all notifications
          </Link>
        </div>
      )}
    </div>
  );
}
