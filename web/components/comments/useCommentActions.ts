"use client";

import { useCallback, useMemo, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { ApiError } from "@/lib/api/client";
import {
  createComment,
  deleteComment,
  reopenComment,
  resolveComment,
  updateComment,
} from "@/lib/api/comments";
import type { Comment, CommentInput, PMDoc } from "@/lib/api/types";
import { useToast } from "@/components/ui/Toast";

export const commentsKey = (pageId: string) => ["comments", pageId] as const;

export interface CommentActions {
  create: (input: CommentInput) => Promise<Comment | null>;
  update: (id: string, value: { body_json: PMDoc | null; body_html: string }) => Promise<boolean>;
  remove: (id: string) => Promise<boolean>;
  setResolved: (id: string, resolved: boolean) => Promise<boolean>;
  pending: boolean;
}

export interface CommentCtx {
  pageId: string;
  meId: string | null;
  canComment: boolean;
  canEdit: boolean;
  actions: CommentActions;
}

export function useCommentActions(pageId: string): CommentActions {
  const qc = useQueryClient();
  const toast = useToast();
  const [pending, setPending] = useState(0);

  const run = useCallback(
    async <T,>(task: () => Promise<T>): Promise<{ ok: true; value: T } | { ok: false }> => {
      setPending((n) => n + 1);
      try {
        const value = await task();
        await qc.invalidateQueries({ queryKey: commentsKey(pageId) });
        return { ok: true, value };
      } catch (e) {
        if (e instanceof ApiError && (e.code === "page_trashed" || e.status === 409)) {
          toast.error("This page is in the trash, so its comments can no longer be changed.");
        } else {
          toast.error(e instanceof Error ? e.message : "Something went wrong");
        }
        await qc.invalidateQueries({ queryKey: commentsKey(pageId) });
        return { ok: false };
      } finally {
        setPending((n) => n - 1);
      }
    },
    [qc, pageId, toast]
  );

  return useMemo<CommentActions>(
    () => ({
      pending: pending > 0,
      create: async (input) => {
        const r = await run(() => createComment(pageId, input));
        return r.ok ? r.value : null;
      },
      update: async (id, value) => (await run(() => updateComment(id, value))).ok,
      remove: async (id) => (await run(() => deleteComment(id))).ok,
      setResolved: async (id, resolved) =>
        (await run(() => (resolved ? resolveComment(id) : reopenComment(id)))).ok,
    }),
    [pending, run, pageId]
  );
}
