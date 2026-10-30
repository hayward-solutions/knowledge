import { api, apiUrl, jsonBody, qs } from "./client";
import type { Notification } from "./types";

export const NOTIFICATIONS_STREAM_PATH = "/notifications/stream";

export interface ListNotificationsParams {
  unread?: boolean;
  before?: string;
  limit?: number;
}

export function listNotifications(
  params: ListNotificationsParams = {}
): Promise<Notification[]> {
  return api<Notification[]>(
    `/notifications${qs({
      unread: params.unread ? "true" : undefined,
      before: params.before,
      limit: params.limit,
    })}`
  );
}

export async function getUnreadCount(): Promise<number> {
  const res = await fetch(apiUrl(`/notifications${qs({ unread: "true", limit: 1 })}`), {
    credentials: "include",
    headers: { Accept: "application/json" },
  });
  if (!res.ok) throw new Error(res.statusText || `Request failed (${res.status})`);
  const header = res.headers.get("X-Unread-Count");
  if (header !== null && header !== "" && !Number.isNaN(Number(header))) {
    return Number(header);
  }
  try {
    const body = (await res.json()) as unknown;
    return Array.isArray(body) ? body.length : 0;
  } catch {
    return 0;
  }
}

export function markNotificationsRead(
  target: { ids: string[] } | { all: true }
): Promise<void> {
  return api<void>("/notifications/read", {
    method: "POST",
    body: jsonBody(target),
  });
}
