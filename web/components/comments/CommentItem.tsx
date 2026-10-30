"use client";

import { useState, type ReactNode } from "react";
import { CornerDownRight } from "lucide-react";
import { ContentView } from "@/components/editor";
import { Avatar } from "@/components/ui/Avatar";
import { ConfirmDialog } from "@/components/ui/ConfirmDialog";
import type { Comment } from "@/lib/api/types";
import { CommentComposer } from "./CommentComposer";
import {
  MAX_REPLY_DEPTH,
  absoluteTime,
  authorName,
  isDeleted,
  relativeTime,
} from "./commentUtils";
import type { CommentCtx } from "./useCommentActions";

export interface CommentItemProps {
  comment: Comment;
  ctx: CommentCtx;
  depth: number;
  replyingTo?: Comment | null;
  onReply?: (comment: Comment) => void;
  extraActions?: ReactNode;
  badge?: ReactNode;
}

function ActionButton({
  onClick,
  children,
  danger,
}: {
  onClick: () => void;
  children: ReactNode;
  danger?: boolean;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={
        danger
          ? "rounded-xs px-1 py-0.5 text-xs font-medium text-ink-600 hover:text-danger-600"
          : "rounded-xs px-1 py-0.5 text-xs font-medium text-ink-600 hover:text-ink-900"
      }
    >
      {children}
    </button>
  );
}

export function CommentItem({
  comment,
  ctx,
  depth,
  replyingTo,
  onReply,
  extraActions,
  badge,
}: CommentItemProps) {
  const [editing, setEditing] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [deleting, setDeleting] = useState(false);

  const deleted = isDeleted(comment);
  const name = authorName(comment);
  const isAuthor = ctx.meId !== null && comment.author_id === ctx.meId;
  const canReply = ctx.canComment && !deleted && depth < MAX_REPLY_DEPTH && Boolean(onReply);
  const canEditBody = ctx.canComment && !deleted && isAuthor;
  const canDelete = ctx.canComment && !deleted && (isAuthor || ctx.canEdit);

  if (deleted) {
    return (
      <div id={`comment-${comment.id}`} className="flex items-start gap-2.5">
        <span className="mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-ink-100 text-ink-400">
          <CornerDownRight size={13} />
        </span>
        <p className="py-1 text-sm italic text-ink-500">This comment was deleted.</p>
      </div>
    );
  }

  async function confirmDelete() {
    setDeleting(true);
    const ok = await ctx.actions.remove(comment.id);
    setDeleting(false);
    if (ok) setConfirming(false);
  }

  return (
    <div id={`comment-${comment.id}`} className="flex items-start gap-2.5">
      <Avatar
        name={name}
        seed={comment.author_id}
        size={28}
        className="mt-0.5"
      />
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-x-2 gap-y-0.5">
          <span className="text-sm font-semibold text-ink-900">{name}</span>
          <time
            dateTime={comment.created_at}
            title={absoluteTime(comment.created_at)}
            className="text-xs text-ink-500"
          >
            {relativeTime(comment.created_at)}
          </time>
          {badge}
        </div>
        {replyingTo && (
          <div className="mt-0.5 flex items-center gap-1 text-xs text-ink-500">
            <CornerDownRight size={11} />
            <span className="truncate">Replying to {authorName(replyingTo)}</span>
          </div>
        )}
        {editing ? (
          <div className="mt-2">
            <CommentComposer
              initial={{ json: comment.body_json, html: comment.body_html }}
              submitLabel="Save"
              autofocus
              ariaLabel="Edit comment"
              onCancel={() => setEditing(false)}
              onSubmit={async (value) => {
                const ok = await ctx.actions.update(comment.id, {
                  body_json: value.json,
                  body_html: value.html,
                });
                if (ok) setEditing(false);
                return ok;
              }}
            />
          </div>
        ) : (
          <>
            <ContentView html={comment.body_html} className="mt-1 text-sm" />
            {(canReply || canEditBody || canDelete || extraActions) && (
              <div className="-ml-1 mt-1.5 flex flex-wrap items-center gap-x-2">
                {canReply && onReply && (
                  <ActionButton onClick={() => onReply(comment)}>Reply</ActionButton>
                )}
                {extraActions}
                {canEditBody && <ActionButton onClick={() => setEditing(true)}>Edit</ActionButton>}
                {canDelete && (
                  <ActionButton danger onClick={() => setConfirming(true)}>
                    Delete
                  </ActionButton>
                )}
              </div>
            )}
          </>
        )}
      </div>
      {confirming && (
        <ConfirmDialog
          title="Delete comment"
          message="This comment will be removed. Replies to it stay in the thread."
          confirmLabel="Delete"
          tone="danger"
          busy={deleting}
          onConfirm={() => void confirmDelete()}
          onCancel={() => setConfirming(false)}
        />
      )}
    </div>
  );
}
