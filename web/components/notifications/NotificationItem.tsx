"use client";

import Link from "next/link";
import { formatDistanceToNowStrict } from "date-fns";
import type { Notification } from "@/lib/api";
import { Avatar } from "@/components/ui/Avatar";
import { cn } from "@/lib/cn";
import {
  notificationActor,
  notificationExcerpt,
  notificationHref,
  notificationPageTitle,
  notificationVerb,
} from "./format";

export function relativeTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return formatDistanceToNowStrict(d, { addSuffix: true });
}

export function notificationSummary(n: Notification): string {
  const actor = notificationActor(n);
  const verb = notificationVerb(n.type);
  if (n.type === "space.added") {
    const key = n.space_key || "";
    return `${actor} ${verb}${key ? ` ${key}` : ""}`;
  }
  return `${actor} ${verb} ${notificationPageTitle(n)}`;
}

export function NotificationItem({
  notification: n,
  spaceKeyById,
  onOpen,
  compact = false,
}: {
  notification: Notification;
  spaceKeyById?: Map<string, string>;
  onOpen?: (n: Notification) => void;
  compact?: boolean;
}) {
  const unread = !n.read_at;
  const actor = notificationActor(n);
  const verb = notificationVerb(n.type);
  const excerpt = notificationExcerpt(n);
  const spaceKey = n.space_key || (n.space_id ? spaceKeyById?.get(n.space_id) : "") || "";
  const href = notificationHref(n, spaceKeyById);

  return (
    <Link
      href={href}
      onClick={() => onOpen?.(n)}
      className={cn(
        "flex gap-3 outline-none transition hover:bg-ink-50 focus-visible:bg-ink-50",
        compact ? "px-3 py-2.5" : "px-4 py-3",
        unread && "bg-brand-50/50"
      )}
    >
      <Avatar name={actor} seed={n.actor_id} size={compact ? 28 : 32} className="mt-0.5 shrink-0" />
      <div className="min-w-0 flex-1">
        <p className="text-sm leading-snug text-ink-800">
          <span className="font-semibold text-ink-900">{actor}</span> {verb}{" "}
          {n.type === "space.added" ? (
            <span className="font-semibold text-ink-900">{spaceKey || "a space"}</span>
          ) : (
            <span className="font-semibold text-brand-700">{notificationPageTitle(n)}</span>
          )}
        </p>
        {excerpt && (
          <p className="mt-0.5 line-clamp-2 text-xs text-ink-600">&ldquo;{excerpt}&rdquo;</p>
        )}
        <p className="mt-0.5 flex items-center gap-1.5 text-xs text-ink-500">
          <time dateTime={n.created_at} title={new Date(n.created_at).toLocaleString()}>
            {relativeTime(n.created_at)}
          </time>
          {spaceKey && n.type !== "space.added" && (
            <>
              <span aria-hidden>&middot;</span>
              <span>{spaceKey}</span>
            </>
          )}
        </p>
      </div>
      {unread && (
        <span
          aria-label="Unread"
          className="mt-1.5 h-2 w-2 shrink-0 rounded-full bg-brand-600"
        />
      )}
    </Link>
  );
}
