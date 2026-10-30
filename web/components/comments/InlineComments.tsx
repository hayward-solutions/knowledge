"use client";

import { useMemo, useState, type RefObject } from "react";
import type { Comment } from "@/lib/api/types";
import { CommentThread } from "./CommentThread";
import { isDeleted, isResolved } from "./commentUtils";
import { useInlineAnchors } from "./useInlineAnchors";
import type { CommentCtx } from "./useCommentActions";

export interface InlineCommentsProps {
  threads: Comment[];
  ctx: CommentCtx;
  contentRef?: RefObject<HTMLElement>;
  contentKey: string;
  activeId: string | null;
  onSelect: (id: string, scroll: boolean) => void;
  onMarkClick: (id: string) => void;
}

export function InlineComments({
  threads,
  ctx,
  contentRef,
  contentKey,
  activeId,
  onSelect,
  onMarkClick,
}: InlineCommentsProps) {
  const [showResolved, setShowResolved] = useState(false);

  const open = useMemo(() => threads.filter((t) => !isResolved(t)), [threads]);
  const resolved = useMemo(() => threads.filter((t) => isResolved(t)), [threads]);

  const anchorThreads = useMemo(
    () =>
      open
        .filter((t) => !isDeleted(t) && t.inline_anchor)
        .map((t) => ({
          id: t.id,
          quote: t.inline_anchor?.quoted_text ?? "",
          prefix: t.inline_anchor?.prefix,
          suffix: t.inline_anchor?.suffix,
        })),
    [open]
  );

  const anchored = useInlineAnchors({
    contentRef,
    threads: anchorThreads,
    activeId,
    contentKey,
    onMarkClick,
  });

  const forcedOpen = resolved.some((t) => t.id === activeId);
  const resolvedVisible = showResolved || forcedOpen;

  if (threads.length === 0) return null;

  function renderThread(t: Comment, orphanCheck: boolean) {
    return (
      <CommentThread
        key={t.id}
        root={t}
        ctx={ctx}
        quote={t.inline_anchor?.quoted_text ?? ""}
        active={activeId === t.id}
        orphaned={orphanCheck && anchored !== null && !anchored.has(t.id)}
        onSelect={onSelect}
      />
    );
  }

  return (
    <aside aria-label="Inline comments" className="min-w-0">
      <h3 className="mb-3 text-sm font-semibold text-ink-900">
        Inline comments ({open.length})
      </h3>
      <div className="space-y-3">
        {open.length === 0 && (
          <p className="text-sm text-ink-500">All inline comments are resolved.</p>
        )}
        {open.map((t) => renderThread(t, !isDeleted(t)))}
      </div>
      {resolved.length > 0 && (
        <div className="mt-4">
          <button
            type="button"
            onClick={() => setShowResolved((v) => !v)}
            aria-expanded={resolvedVisible}
            className="text-xs font-medium text-ink-600 hover:text-ink-900"
          >
            {resolvedVisible ? "Hide" : "Show"} {resolved.length} resolved
          </button>
          {resolvedVisible && (
            <div className="mt-3 space-y-3">{resolved.map((t) => renderThread(t, false))}</div>
          )}
        </div>
      )}
    </aside>
  );
}
