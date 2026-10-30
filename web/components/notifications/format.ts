import type { Notification, UserRef } from "@/lib/api";
import { slugify } from "@/lib/content";

type Payload = Record<string, unknown>;

export function payloadString(payload: Payload | null | undefined, ...keys: string[]): string {
  if (!payload || typeof payload !== "object") return "";
  for (const k of keys) {
    const v = payload[k];
    if (typeof v === "string" && v.trim()) return v.trim();
    if (typeof v === "number") return String(v);
  }
  return "";
}

export function payloadNumber(payload: Payload | null | undefined, key: string): number | null {
  if (!payload || typeof payload !== "object") return null;
  const v = payload[key];
  if (typeof v === "number") return v;
  if (typeof v === "string" && v.trim() !== "" && !Number.isNaN(Number(v))) return Number(v);
  return null;
}

export function actorNameOf(
  actor: UserRef | null | undefined,
  payload: Payload | null | undefined
): string {
  return (
    actor?.display_name ||
    payloadString(payload, "actor_name", "actor_display_name", "actor") ||
    "Someone"
  );
}

export function notificationActor(n: Notification): string {
  return actorNameOf(n.actor, n.payload);
}

export function notificationPageTitle(n: Notification): string {
  return n.page_title || payloadString(n.payload, "page_title", "title") || "Untitled";
}

export function notificationSpaceKey(n: Notification): string {
  return n.space_key || payloadString(n.payload, "space_key");
}

export function notificationExcerpt(n: Notification): string {
  return payloadString(n.payload, "comment_excerpt", "excerpt", "snippet");
}

export function notificationVerb(type: string): string {
  switch (type) {
    case "page.published":
      return "published";
    case "page.mentioned":
      return "mentioned you in";
    case "comment.created":
      return "commented on";
    case "comment.replied":
      return "replied to your comment on";
    case "comment.mentioned":
      return "mentioned you in a comment on";
    case "comment.resolved":
      return "resolved a comment on";
    case "space.added":
      return "added you to the space";
    default:
      return type.replace(/[._]/g, " ");
  }
}

export function pageHref(spaceKey: string, pageId: string, title: string, commentId?: string | null): string {
  const base = `/spaces/${spaceKey}/pages/${pageId}/${slugify(title)}`;
  return commentId ? `${base}#comment-${commentId}` : base;
}

export function notificationHref(n: Notification, spaceKeyById?: Map<string, string>): string {
  const key =
    notificationSpaceKey(n) || (n.space_id ? spaceKeyById?.get(n.space_id) ?? "" : "");
  if (!key) return "/notifications";
  if (n.type === "space.added" || !n.page_id) return `/spaces/${key}`;
  const slug = payloadString(n.payload, "page_slug");
  const title = notificationPageTitle(n);
  const base = `/spaces/${key}/pages/${n.page_id}/${slug || slugify(title)}`;
  return n.comment_id ? `${base}#comment-${n.comment_id}` : base;
}

export function isNotificationLike(v: unknown): v is Notification {
  if (!v || typeof v !== "object") return false;
  const o = v as Record<string, unknown>;
  return typeof o.id === "string" && typeof o.type === "string";
}

export function unreadCountFrom(data: unknown): number | null {
  if (typeof data === "number" && Number.isFinite(data)) return data;
  if (typeof data === "string" && data.trim() !== "" && !Number.isNaN(Number(data))) {
    return Number(data);
  }
  if (data && typeof data === "object") {
    const o = data as Record<string, unknown>;
    for (const k of ["count", "unread_count", "unread", "total"]) {
      const v = o[k];
      if (typeof v === "number" && Number.isFinite(v)) return v;
    }
  }
  return null;
}
