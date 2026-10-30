"use client";

import { format, formatDistanceToNow } from "date-fns";

export function relativeTime(iso: string | null | undefined): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return formatDistanceToNow(d, { addSuffix: true });
}

export function fullDate(iso: string | null | undefined): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return format(d, "PPpp");
}

export function RelativeTime({ iso, className }: { iso: string | null | undefined; className?: string }) {
  if (!iso) return null;
  return (
    <time dateTime={iso} title={fullDate(iso)} className={className}>
      {relativeTime(iso)}
    </time>
  );
}
