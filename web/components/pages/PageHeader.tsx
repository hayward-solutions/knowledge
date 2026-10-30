"use client";

import Link from "next/link";
import { Pencil, Users } from "lucide-react";
import type { PageDetail, UserRef } from "@/lib/api";
import { Avatar } from "@/components/ui/Avatar";
import { Badge } from "@/components/ui/Badge";
import { RelativeTime } from "./RelativeTime";
import { pageEditHref, pageHistoryHref } from "./utils";

function joinNames(users: UserRef[]): string {
  const names = users.map((u) => u.display_name);
  if (names.length <= 1) return names[0] ?? "";
  if (names.length === 2) return `${names[0]} and ${names[1]}`;
  return `${names.slice(0, -1).join(", ")} and ${names[names.length - 1]}`;
}

export function PageHeader({
  page,
  spaceKey,
  canEdit,
}: {
  page: PageDetail;
  spaceKey: string;
  canEdit: boolean;
}) {
  const creator = page.created_by_user;
  const updater = page.updated_by_user ?? creator;
  const trashed = page.status === "trashed";
  const draft = page.status === "draft";
  const editors = page.editors ?? [];

  return (
    <div>
      <div className="flex flex-wrap items-start gap-x-3 gap-y-2">
        <h1 className="min-w-0 flex-1 break-words text-[28px] font-semibold leading-tight tracking-tight text-ink-900 sm:text-[32px]">
          {page.title || "Untitled"}
        </h1>
        <div className="mt-2 flex shrink-0 items-center gap-1.5">
          {draft && <Badge tone="amber" bold>Draft</Badge>}
          {trashed && <Badge tone="red" bold>Trashed</Badge>}
        </div>
      </div>

      <div className="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1.5 text-sm text-ink-600">
        {creator && (
          <span className="flex items-center gap-1.5">
            <Avatar name={creator.display_name} seed={creator.id} size={20} />
            <span>
              Created by <span className="font-medium text-ink-800">{creator.display_name}</span>
            </span>
          </span>
        )}
        {updater && page.version > 0 && (
          <span>
            Last updated by <span className="font-medium text-ink-800">{updater.display_name}</span>{" "}
            <RelativeTime iso={page.updated_at} />
          </span>
        )}
        {!updater && page.version > 0 && (
          <span>
            Last updated <RelativeTime iso={page.updated_at} />
          </span>
        )}
        {page.version > 0 && (
          <Link
            href={pageHistoryHref(spaceKey, page.id)}
            className="link"
            title="View version history"
          >
            Version {page.version}
          </Link>
        )}
        {draft && page.version === 0 && <span>Not published yet</span>}
        {trashed && page.trashed_at && (
          <span>
            Moved to trash <RelativeTime iso={page.trashed_at} />
          </span>
        )}
      </div>

      {page.my_draft && !trashed && canEdit && (
        <div
          role="status"
          className="mt-4 flex flex-wrap items-center gap-2 rounded-md border border-brand-100 bg-brand-50 px-3 py-2 text-sm text-brand-700 print:hidden"
        >
          <Pencil size={14} className="shrink-0" />
          <span className="min-w-0 flex-1">
            You have unpublished changes to this page (saved <RelativeTime iso={page.my_draft.updated_at} />
            ).
          </span>
          <Link href={pageEditHref(spaceKey, page.id)} className="font-semibold underline">
            Continue editing
          </Link>
        </div>
      )}

      {editors.length > 0 && !trashed && (
        <div
          role="status"
          className="mt-3 flex items-center gap-2 rounded-md border border-warning-100 bg-warning-50 px-3 py-2 text-sm text-warning-700 print:hidden"
        >
          <Users size={14} className="shrink-0" />
          <span>
            {editors.length === 1
              ? `${joinNames(editors)} is editing this page right now.`
              : `${editors.length} people are editing this page right now: ${joinNames(editors)}.`}
          </span>
        </div>
      )}
    </div>
  );
}
