import { formatDistanceToNowStrict } from "date-fns";
import type { Comment } from "@/lib/api/types";

export const MAX_REPLY_DEPTH = 2;

export function isDeleted(c: Comment): boolean {
  return Boolean(c.deleted_at);
}

export function isResolved(c: Comment): boolean {
  return Boolean(c.resolved_at);
}

export function countLive(list: Comment[]): number {
  let n = 0;
  for (const c of list) {
    if (!isDeleted(c)) n += 1;
    n += countLive(c.replies ?? []);
  }
  return n;
}

export interface FlatReply {
  comment: Comment;
  depth: number;
  parent: Comment;
}

export function flattenReplies(root: Comment): FlatReply[] {
  const out: FlatReply[] = [];
  const walk = (parent: Comment, depth: number) => {
    for (const reply of parent.replies ?? []) {
      out.push({ comment: reply, depth, parent });
      walk(reply, depth + 1);
    }
  };
  walk(root, 1);
  return out;
}

export function relativeTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return formatDistanceToNowStrict(d, { addSuffix: true });
}

export function absoluteTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return d.toLocaleString();
}

export function authorName(c: Comment): string {
  return c.author?.display_name || "Unknown user";
}

export function normalizeQuote(raw: string): string {
  return raw.replace(/\s+/g, " ").trim();
}

export const MAX_QUOTE_LENGTH = 2000;
