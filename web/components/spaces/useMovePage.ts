"use client";

import { useCallback } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { movePage, type PageSummary } from "@/lib/api";
import { useToast } from "@/components/ui/Toast";
import { useSpace } from "./SpaceContext";
import { applyMove, type MoveTarget } from "./tree";
import { errorMessage } from "@/components/pages/utils";

export function useMovePage() {
  const { space } = useSpace();
  const qc = useQueryClient();
  const toast = useToast();
  const key = space.key;

  return useCallback(
    async (id: string, target: MoveTarget): Promise<boolean> => {
      const treeKey = ["tree", key];
      await qc.cancelQueries({ queryKey: treeKey });
      const prev = qc.getQueryData<PageSummary[]>(treeKey);
      if (prev) qc.setQueryData<PageSummary[]>(treeKey, applyMove(prev, id, target));
      try {
        await movePage(id, {
          parent_id: target.parentId,
          after_id: target.afterId ?? undefined,
          position: target.position,
        });
        return true;
      } catch (err) {
        if (prev) qc.setQueryData<PageSummary[]>(treeKey, prev);
        toast.error(errorMessage(err, "Could not move page"));
        return false;
      } finally {
        void qc.invalidateQueries({ queryKey: treeKey });
        void qc.invalidateQueries({ queryKey: ["page"] });
        void qc.invalidateQueries({ queryKey: ["dashboard"] });
      }
    },
    [qc, key, toast]
  );
}
