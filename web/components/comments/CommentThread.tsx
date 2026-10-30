"use client";

import { useMemo, useState } from "react";
import { Check, RotateCcw } from "lucide-react";
import { Badge } from "@/components/ui/Badge";
import { cn } from "@/lib/cn";
import type { Comment } from "@/lib/api/types";
import { CommentComposer } from "./CommentComposer";
import { CommentItem } from "./CommentItem";
import { flattenReplies, isDeleted, isResolved, relativeTime } from "./commentUtils";
import type { CommentCtx } from "./useCommentActions";

export interface CommentThreadProps {
  root: Comment;
  ctx: CommentCtx;
  quote?: string;
  active?: boolean;
  orphaned?: boolean;
  onSelect?: (id: string, scroll: boolean) => void;
}

export function CommentThread({
  root,
  ctx,
  quote,
  active = false,
  orphaned = false,
  onSelect,
}: CommentThreadProps) {
  const [replyTo, setReplyTo] = useState<Comment | null>(null);
  const [resolving, setResolving] = useState(false);
  const replies = useMemo(() => flattenReplies(root), [root]);
  const resolved = isResolved(root);
  const rootDeleted = isDeleted(root);

  async function toggleResolved() {
    setResolving(true);
    await ctx.actions.setResolved(root.id, !resolved);
    setResolving(false);
  }

  const resolveButton =
    ctx.canComment && !rootDeleted ? (
      <button
        type="button"
        onClick={() => void toggleResolved()}
        disabled={resolving}
        className="inline-flex items-center gap-1 rounded-xs px-1 py-0.5 text-xs font-medium text-ink-600 hover:text-ink-900 disabled:opacity-50"
      >
        {resolved ? <RotateCcw size={12} /> : <Check size={12} />}
        {resolved ? "Reopen" : "Resolve"}
      </button>
    ) : null;

  return (
    <article
      data-comment-thread={root.id}
      onClick={onSelect ? () => onSelect(root.id, false) : undefined}
      className={cn(
        "surface p-3 transition-colors",
        resolved && "bg-ink-50",
        active && "border-brand-400 ring-2 ring-brand-400/40"
      )}
    >
      {quote !== undefined && (
        <div className="mb-2">
          {onSelect ? (
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation();
                onSelect(root.id, true);
              }}
              title="Jump to the commented text"
              className="block w-full rounded-xs border-l-2 border-warning-500 bg-warning-50 px-2 py-1 text-left text-xs text-ink-700 hover:bg-warning-100"
            >
              <span className="line-clamp-3 break-words">{quote}</span>
            </button>
          ) : (
            <blockquote className="rounded-xs border-l-2 border-warning-500 bg-warning-50 px-2 py-1 text-xs text-ink-700">
              <span className="line-clamp-3 break-words">{quote}</span>
            </blockquote>
          )}
          {orphaned && (
            <p className="mt-1 text-[11px] italic text-ink-500">
              The commented text is no longer present on this page.
            </p>
          )}
        </div>
      )}

      <CommentItem
        comment={root}
        ctx={ctx}
        depth={0}
        onReply={(c) => setReplyTo(c)}
        extraActions={resolveButton}
        badge={
          resolved ? (
            <Badge tone="green" title={root.resolved_at ? new Date(root.resolved_at).toLocaleString() : undefined}>
              Resolved {root.resolved_at ? relativeTime(root.resolved_at) : ""}
            </Badge>
          ) : undefined
        }
      />

      {(replies.length > 0 || replyTo) && (
        <div className="ml-3.5 mt-3 space-y-3 border-l border-ink-200 pl-4 sm:ml-[13px]">
          {replies.map(({ comment, depth, parent }) => (
            <CommentItem
              key={comment.id}
              comment={comment}
              ctx={ctx}
              depth={depth}
              replyingTo={parent.id === root.id ? null : parent}
              onReply={(c) => setReplyTo(c)}
            />
          ))}
          {replyTo && (
            <div>
              <p className="mb-1 text-xs text-ink-500">
                Replying to {replyTo.author?.display_name || "this comment"}
              </p>
              <CommentComposer
                autofocus
                placeholder="Write a reply"
                submitLabel="Reply"
                ariaLabel="Reply"
                onCancel={() => setReplyTo(null)}
                onSubmit={async (value) => {
                  const created = await ctx.actions.create({
                    body_json: value.json,
                    body_html: value.html,
                    parent_id: replyTo.id,
                  });
                  if (created) setReplyTo(null);
                  return Boolean(created);
                }}
              />
            </div>
          )}
        </div>
      )}
    </article>
  );
}
