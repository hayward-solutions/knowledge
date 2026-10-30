"use client";

import Link from "next/link";
import {
  FilePlus2,
  FolderInput,
  MessageSquare,
  RotateCcw,
  Trash2,
  UserPlus,
  type LucideIcon,
} from "lucide-react";
import { format } from "date-fns";
import type { Activity } from "@/lib/api";
import { slugify } from "@/lib/content";
import { Avatar } from "@/components/ui/Avatar";
import { actorNameOf, payloadNumber, payloadString } from "@/components/notifications/format";
import { relativeTime } from "@/components/notifications/NotificationItem";

interface Descriptor {
  icon: LucideIcon;
  tone: string;
  verb: string;
}

function describe(type: string): Descriptor {
  switch (type) {
    case "page.published":
      return { icon: FilePlus2, tone: "bg-success-50 text-success-700", verb: "published" };
    case "page.moved":
      return { icon: FolderInput, tone: "bg-brand-50 text-brand-700", verb: "moved" };
    case "page.trashed":
      return { icon: Trash2, tone: "bg-danger-50 text-danger-700", verb: "moved to trash" };
    case "page.restored":
      return { icon: RotateCcw, tone: "bg-warning-50 text-warning-700", verb: "restored" };
    case "comment.created":
      return { icon: MessageSquare, tone: "bg-purple-50 text-purple-700", verb: "commented on" };
    case "comment.resolved":
      return { icon: MessageSquare, tone: "bg-success-50 text-success-700", verb: "resolved a comment on" };
    case "space.member_added":
    case "space.added":
      return { icon: UserPlus, tone: "bg-teal-50 text-teal-600", verb: "added a member to the space" };
    default:
      return { icon: FilePlus2, tone: "bg-ink-100 text-ink-700", verb: type.replace(/[._]/g, " ") };
  }
}

export function activityDay(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return format(d, "EEEE, d MMMM yyyy");
}

export function ActivityItem({ activity: a, spaceKey }: { activity: Activity; spaceKey: string }) {
  const d = describe(a.type);
  const Icon = d.icon;
  const actor = actorNameOf(a.actor, a.payload);
  const title = a.page_title || payloadString(a.payload, "page_title", "title");
  const version = payloadNumber(a.payload, "version");
  const message = a.type === "page.published" ? payloadString(a.payload, "message") : "";
  const excerpt =
    a.type === "comment.created" || a.type === "comment.resolved"
      ? payloadString(a.payload, "comment_excerpt", "excerpt", "snippet")
      : "";
  const commentId = payloadString(a.payload, "comment_id");
  const trashed = a.type === "page.trashed";

  let href: string | null = null;
  if (a.page_id) {
    href = trashed
      ? `/spaces/${spaceKey}/trash`
      : `/spaces/${spaceKey}/pages/${a.page_id}/${payloadString(a.payload, "page_slug") || slugify(title || "page")}${
          commentId ? `#comment-${commentId}` : ""
        }`;
  }

  const pageNode = title ? (
    href ? (
      <Link href={href} className="font-semibold text-brand-700 hover:underline">
        {title}
      </Link>
    ) : (
      <span className="font-semibold text-ink-900">{title}</span>
    )
  ) : null;

  return (
    <div className="flex gap-3 px-4 py-3">
      <span
        className={`mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-full ${d.tone}`}
        aria-hidden
      >
        <Icon size={15} />
      </span>
      <div className="min-w-0 flex-1">
        <p className="text-sm leading-snug text-ink-800">
          <span className="inline-flex items-center gap-1.5 align-middle">
            <Avatar name={actor} seed={a.actor_id} size={18} />
            <span className="font-semibold text-ink-900">{actor}</span>
          </span>{" "}
          {d.verb} {pageNode}
          {version !== null && a.type === "page.published" && (
            <span className="ml-1.5 rounded bg-ink-100 px-1.5 py-0.5 text-xs font-medium text-ink-700">
              v{version}
            </span>
          )}
        </p>
        {message && <p className="mt-0.5 text-sm text-ink-600">&ldquo;{message}&rdquo;</p>}
        {excerpt && <p className="mt-0.5 line-clamp-2 text-sm text-ink-600">&ldquo;{excerpt}&rdquo;</p>}
        <time
          dateTime={a.created_at}
          title={new Date(a.created_at).toLocaleString()}
          className="mt-0.5 block text-xs text-ink-500"
        >
          {relativeTime(a.created_at)}
        </time>
      </div>
    </div>
  );
}
