"use client";

import { useCallback, useEffect, useMemo, useRef, useState, type RefObject } from "react";
import { useQuery } from "@tanstack/react-query";
import type { EditorChange } from "@/components/editor";
import { listComments } from "@/lib/api/comments";
import type { PageDetail } from "@/lib/api/types";
import { Button } from "@/components/ui/Button";
import { Spinner } from "@/components/ui/Spinner";
import { useMe } from "@/lib/auth";
import { cn } from "@/lib/cn";
import { CommentComposer } from "./CommentComposer";
import { CommentThread } from "./CommentThread";
import { InlineCommentDialog } from "./InlineCommentDialog";
import { InlineComments } from "./InlineComments";
import { SelectionCommentButton } from "./SelectionCommentButton";
import { scrollMarkIntoView } from "./anchors";
import { countLive, isResolved } from "./commentUtils";
import { commentsKey, useCommentActions, type CommentCtx } from "./useCommentActions";

export interface PageCommentsProps {
  page: PageDetail;
  canComment: boolean;
  canEdit: boolean;
  contentRef?: RefObject<HTMLElement>;
  activeCommentId?: string;
  onActiveCommentChange?: (id: string | null) => void;
}

export function PageComments({
  page,
  canComment,
  canEdit,
  contentRef,
  activeCommentId,
  onActiveCommentChange,
}: PageCommentsProps) {
  const { data: me } = useMe();
  const actions = useCommentActions(page.id);
  const trashed = page.status === "trashed";
  const commentingAllowed = canComment && !trashed;

  const query = useQuery({
    queryKey: commentsKey(page.id),
    queryFn: () => listComments(page.id),
  });

  const [active, setActive] = useState<string | null>(activeCommentId ?? null);
  const internalRef = useRef<string | null>(null);
  const [showResolved, setShowResolved] = useState(false);
  const [dialogQuote, setDialogQuote] = useState<string | null>(null);
  const changeRef = useRef(onActiveCommentChange);

  useEffect(() => {
    changeRef.current = onActiveCommentChange;
  });

  useEffect(() => {
    setActive(activeCommentId ?? null);
    if (activeCommentId && activeCommentId !== internalRef.current) {
      document
        .getElementById(`comment-${activeCommentId}`)
        ?.scrollIntoView({ behavior: "smooth", block: "center" });
    }
    internalRef.current = null;
  }, [activeCommentId]);

  const select = useCallback((id: string | null) => {
    internalRef.current = id;
    setActive(id);
    changeRef.current?.(id);
  }, []);

  const selectFromThread = useCallback(
    (id: string, scroll: boolean) => {
      select(id);
      const root = contentRef?.current;
      if (scroll && root) scrollMarkIntoView(root, id);
    },
    [select, contentRef]
  );

  const selectFromMark = useCallback(
    (id: string) => {
      select(id);
      document
        .getElementById(`comment-${id}`)
        ?.scrollIntoView({ behavior: "smooth", block: "center" });
    },
    [select]
  );

  const ctx = useMemo<CommentCtx>(
    () => ({
      pageId: page.id,
      meId: me?.id ?? null,
      canComment: commentingAllowed,
      canEdit,
      actions,
    }),
    [page.id, me?.id, commentingAllowed, canEdit, actions]
  );

  const comments = useMemo(() => query.data ?? [], [query.data]);
  const pageThreads = useMemo(() => comments.filter((c) => !c.inline_anchor), [comments]);
  const inlineThreads = useMemo(() => comments.filter((c) => c.inline_anchor), [comments]);
  const openThreads = useMemo(() => pageThreads.filter((c) => !isResolved(c)), [pageThreads]);
  const resolvedThreads = useMemo(() => pageThreads.filter((c) => isResolved(c)), [pageThreads]);
  const total = useMemo(() => countLive(pageThreads), [pageThreads]);

  const hashHandled = useRef(false);
  useEffect(() => {
    if (hashHandled.current || !query.data) return;
    hashHandled.current = true;
    const match = /^#comment-(.+)$/.exec(window.location.hash);
    if (!match) return;
    const id = match[1];
    const inline = query.data.find((c) => c.id === id && c.inline_anchor);
    if (inline) select(id);
    window.setTimeout(() => {
      document.getElementById(`comment-${id}`)?.scrollIntoView({ block: "center" });
    }, 150);
  }, [query.data, select]);

  const hasInline = inlineThreads.length > 0;
  const resolvedActive = resolvedThreads.some((t) => t.id === active);
  const resolvedVisible = showResolved || resolvedActive;

  const submitPageComment = useCallback(
    async (value: EditorChange) => {
      const created = await actions.create({ body_json: value.json, body_html: value.html });
      return Boolean(created);
    },
    [actions]
  );

  const submitInline = useCallback(
    async (quote: string, value: EditorChange) => {
      const created = await actions.create({
        body_json: value.json,
        body_html: value.html,
        inline_anchor: { quoted_text: quote },
      });
      if (created) {
        select(created.id);
        setDialogQuote(null);
        window.setTimeout(() => {
          document
            .getElementById(`comment-${created.id}`)
            ?.scrollIntoView({ behavior: "smooth", block: "nearest" });
        }, 150);
      }
      return Boolean(created);
    },
    [actions, select]
  );

  return (
    <section
      id="comments"
      aria-labelledby="page-comments-heading"
      className="mt-10 border-t border-ink-200 pt-6"
    >
      <h2 id="page-comments-heading" className="mb-4 text-lg font-semibold text-ink-900">
        Comments ({total})
      </h2>

      {trashed && (
        <p className="mb-4 rounded-md border border-warning-100 bg-warning-50 px-3 py-2 text-sm text-warning-700">
          This page is in the trash, so comments are read-only.
        </p>
      )}

      {query.isPending && (
        <div className="flex items-center gap-2 py-6 text-sm text-ink-500">
          <Spinner size={14} /> Loading comments
        </div>
      )}

      {query.isError && (
        <div className="rounded-md border border-danger-100 bg-danger-50 px-3 py-3 text-sm text-danger-700">
          <p>
            {query.error instanceof Error ? query.error.message : "Could not load comments"}
          </p>
          <Button size="sm" className="mt-2" onClick={() => void query.refetch()}>
            Retry
          </Button>
        </div>
      )}

      {query.data !== undefined && (
        <div className={cn(hasInline && "gap-8 lg:grid lg:grid-cols-[minmax(0,1fr)_340px]")}>
          <div className="min-w-0">
            <div className="space-y-3">
              {openThreads.length === 0 && resolvedThreads.length === 0 && (
                <p className="text-sm text-ink-500">
                  {commentingAllowed
                    ? "No comments yet. Start the conversation below."
                    : "No comments yet."}
                </p>
              )}
              {openThreads.map((t) => (
                <CommentThread key={t.id} root={t} ctx={ctx} />
              ))}
            </div>

            {resolvedThreads.length > 0 && (
              <div className="mt-4">
                <button
                  type="button"
                  aria-expanded={resolvedVisible}
                  onClick={() => setShowResolved((v) => !v)}
                  className="text-xs font-medium text-ink-600 hover:text-ink-900"
                >
                  {resolvedVisible ? "Hide" : "Show"} {resolvedThreads.length} resolved
                </button>
                {resolvedVisible && (
                  <div className="mt-3 space-y-3">
                    {resolvedThreads.map((t) => (
                      <CommentThread key={t.id} root={t} ctx={ctx} />
                    ))}
                  </div>
                )}
              </div>
            )}

            <div className="mt-6">
              <CommentComposer
                placeholder="Write a comment. Type @ to mention someone."
                submitLabel="Comment"
                disabled={!commentingAllowed}
                disabledMessage={
                  trashed
                    ? "Comments are read-only while the page is in the trash."
                    : "You do not have permission to comment on this page."
                }
                onSubmit={submitPageComment}
              />
              {commentingAllowed && contentRef && (
                <p className="mt-2 text-xs text-ink-500">
                  Tip: select text in the page to leave an inline comment.
                </p>
              )}
            </div>
          </div>

          <div
            className={cn(
              "min-w-0 lg:mt-0 lg:sticky lg:top-20 lg:max-h-[calc(100vh-6rem)] lg:self-start lg:overflow-y-auto lg:pr-1",
              hasInline ? "mt-8" : "hidden"
            )}
          >
            <InlineComments
              threads={inlineThreads}
              ctx={ctx}
              contentRef={contentRef}
              contentKey={page.content_html}
              activeId={active}
              onSelect={selectFromThread}
              onMarkClick={selectFromMark}
            />
          </div>
        </div>
      )}

      <SelectionCommentButton
        contentRef={contentRef}
        enabled={commentingAllowed && Boolean(contentRef)}
        onRequest={setDialogQuote}
      />

      {dialogQuote !== null && (
        <InlineCommentDialog
          quote={dialogQuote}
          onClose={() => setDialogQuote(null)}
          onSubmit={(value) => submitInline(dialogQuote, value)}
        />
      )}
    </section>
  );
}
