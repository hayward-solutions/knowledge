import { api, jsonBody, qs } from "./client";
import type { Notification } from "./types";

export const NOTIFICATIONS_STREAM_PATH = "/notifications/stream";

export function listNotifications(
  params: { unread?: boolean } = {}
): Promise<Notification[]> {
  return api<Notification[]>(
    `/notifications${qs({ unread: params.unread ? "true" : undefined })}`
  );
}

export function markNotificationsRead(
  target: { ids: string[] } | { all: true }
): Promise<void> {
  return api<void>("/notifications/read", {
    method: "POST",
    body: jsonBody(target),
  });
}
