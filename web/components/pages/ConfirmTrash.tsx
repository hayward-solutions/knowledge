"use client";

import { useMemo, useState } from "react";
import { usePathname, useRouter } from "next/navigation";
import { useQueryClient } from "@tanstack/react-query";
import { trashPage } from "@/lib/api";
import { ConfirmDialog } from "@/components/ui/ConfirmDialog";
import { useToast } from "@/components/ui/Toast";
import { useSpace } from "@/components/spaces/SpaceContext";
import { ancestorIds, descendantIds } from "@/components/spaces/tree";
import { errorMessage, pageHref } from "./utils";

export function ConfirmTrash({
  pageId,
  onClose,
  onTrashed,
}: {
  pageId: string;
  onClose: () => void;
  onTrashed?: () => void;
}) {
  const router = useRouter();
  const pathname = usePathname();
  const qc = useQueryClient();
  const toast = useToast();
  const { space, tree } = useSpace();
  const [busy, setBusy] = useState(false);
  const page = tree.find((p) => p.id === pageId);
  const count = useMemo(() => descendantIds(tree, pageId).size, [tree, pageId]);

  async function confirm() {
    setBusy(true);
    try {
      await trashPage(pageId);
      const inside = pathname.includes(`/pages/${pageId}`);
      const descendants = descendantIds(tree, pageId);
      const viewingDescendant = Array.from(descendants).some((id) => pathname.includes(`/pages/${id}`));
      toast.success(count > 0 ? `Page and ${count} child ${count === 1 ? "page" : "pages"} moved to trash` : "Page moved to trash");
      await Promise.all([
        qc.invalidateQueries({ queryKey: ["tree", space.key] }),
        qc.invalidateQueries({ queryKey: ["page"] }),
        qc.invalidateQueries({ queryKey: ["trash", space.key] }),
        qc.invalidateQueries({ queryKey: ["dashboard"] }),
      ]);
      onTrashed?.();
      onClose();
      if (inside || viewingDescendant) {
        const parentId = ancestorIds(tree, pageId)[0];
        const parent = parentId ? tree.find((p) => p.id === parentId) : undefined;
        router.push(parent ? pageHref(space.key, parent) : `/spaces/${space.key}`);
      }
    } catch (err) {
      toast.error(errorMessage(err, "Could not move page to trash"));
      setBusy(false);
    }
  }

  return (
    <ConfirmDialog
      title="Move to trash"
      tone="danger"
      confirmLabel="Move to trash"
      busy={busy}
      onConfirm={confirm}
      onCancel={onClose}
      message={
        <>
          <p>
            <strong>{page?.title || "This page"}</strong>
            {count > 0
              ? ` and its ${count} child ${count === 1 ? "page" : "pages"} will be moved to the trash.`
              : " will be moved to the trash."}
          </p>
          <p className="mt-2 text-ink-600">Editors can restore trashed pages from the space trash.</p>
        </>
      }
    />
  );
}
