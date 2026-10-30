"use client";

import { createContext, useContext, useMemo, type ReactNode } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { NOTIFICATIONS_STREAM_PATH, getUnreadCount } from "@/lib/api";
import { useMe } from "@/lib/auth";
import { useEventStream } from "@/lib/sse";
import { useToast } from "@/components/ui/Toast";
import { notificationSummary } from "./NotificationItem";
import { isNotificationLike, unreadCountFrom } from "./format";

export const UNREAD_KEY = ["unread"] as const;

interface NotificationsContextValue {
  unread: number;
}

const NotificationsContext = createContext<NotificationsContextValue>({ unread: 0 });

export function useNotifications(): NotificationsContextValue {
  return useContext(NotificationsContext);
}

export function NotificationsProvider({ children }: { children: ReactNode }) {
  const qc = useQueryClient();
  const toast = useToast();
  const me = useMe();
  const signedIn = Boolean(me.data) && !me.isError;

  const unread = useQuery({
    queryKey: UNREAD_KEY,
    queryFn: getUnreadCount,
    enabled: signedIn,
    staleTime: 30_000,
    retry: false,
  });

  const syncUnread = (data: unknown) => {
    const n = unreadCountFrom(data);
    if (n !== null) qc.setQueryData(UNREAD_KEY, n);
    else qc.invalidateQueries({ queryKey: UNREAD_KEY });
  };

  useEventStream(signedIn ? NOTIFICATIONS_STREAM_PATH : null, {
    hello: (data: unknown) => {
      syncUnread(data);
      qc.invalidateQueries({ queryKey: ["notifications"] });
    },
    unread_count: syncUnread,
    notification: (data: unknown) => {
      qc.invalidateQueries({ queryKey: ["notifications"] });
      qc.invalidateQueries({ queryKey: UNREAD_KEY });
      toast.info(isNotificationLike(data) ? notificationSummary(data) : "You have a new notification");
    },
  });

  const value = useMemo<NotificationsContextValue>(
    () => ({ unread: signedIn ? unread.data ?? 0 : 0 }),
    [signedIn, unread.data]
  );

  return <NotificationsContext.Provider value={value}>{children}</NotificationsContext.Provider>;
}
