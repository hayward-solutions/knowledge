"use client";

import { useState } from "react";
import Link from "next/link";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { FileText, Lock, RotateCcw, Trash2 } from "lucide-react";
import {
  ApiError,
  getSpaceTrash,
  purgePage,
  restorePage,
  type TrashedPage,
} from "@/lib/api";
import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { ConfirmDialog } from "@/components/ui/ConfirmDialog";
import { EmptyState } from "@/components/ui/EmptyState";
import { useToast } from "@/components/ui/Toast";
import { RelativeTime } from "@/components/pages/RelativeTime";
import { ListSkeleton } from "@/components/pages/Skeletons";
import { errorMessage, pageHref } from "@/components/pages/utils";
import { useSpace } from "@/components/spaces/SpaceContext";

export default function TrashPage() {
  const qc = useQueryClient();
  const toast = useToast();
  const { space, isEditor, isAdmin, canEdit, refresh } = useSpace();
  const [toPurge, setToPurge] = useState<TrashedPage | null>(null);
  const key = space.key;

  const query = useQuery<TrashedPage[], Error>({
    queryKey: ["trash", key],
    queryFn: () => getSpaceTrash(key),
    enabled: isEditor,
    retry: false,
  });

  async function afterChange() {
    await Promise.all([
      qc.invalidateQueries({ queryKey: ["trash", key] }),
      qc.invalidateQueries({ queryKey: ["page"] }),
      qc.invalidateQueries({ queryKey: ["dashboard"] }),
      refresh(),
    ]);
  }

  const restore = useMutation({
    mutationFn: (p: TrashedPage) => restorePage(p.id),
    onSuccess: async (_page, p) => {
      toast.success(`Restored "${p.title || "Untitled"}"`);
      await afterChange();
    },
    onError: (err) => toast.error(errorMessage(err, "Could not restore page")),
  });

  const purge = useMutation({
    mutationFn: (p: TrashedPage) => purgePage(p.id),
    onSuccess: async (_v, p) => {
      toast.success(`Permanently deleted "${p.title || "Untitled"}"`);
      setToPurge(null);
      qc.removeQueries({ queryKey: ["page", p.id] });
      await afterChange();
    },
    onError: (err) => {
      setToPurge(null);
      toast.error(errorMessage(err, "Could not delete page"));
    },
  });

  const forbidden =
    !isEditor || (query.error instanceof ApiError && (query.error.status === 403 || query.error.status === 404));

  return (
    <div className="flex-1 bg-ink-0 px-4 py-6 sm:px-8">
      <div className="mx-auto w-full max-w-4xl">
        <div className="flex items-center gap-2">
          <Trash2 size={20} className="text-ink-500" />
          <h1 className="text-[22px] font-semibold tracking-tight text-ink-900">Trash</h1>
        </div>
        <p className="mt-1 text-sm text-ink-600">
          Pages moved to the trash in {space.name}. Restoring a page also restores the pages beneath it.
        </p>

        <div className="mt-6">
          {forbidden ? (
            <EmptyState
              icon={<Lock size={18} />}
              title="Trash is available to editors"
              description="Ask a space admin to give you the editor role."
              action={
                <Link href={`/spaces/${key}`} className="btn btn-primary">
                  Back to space
                </Link>
              }
            />
          ) : query.isPending ? (
            <ListSkeleton rows={4} />
          ) : query.isError ? (
            <EmptyState
              title="Could not load the trash"
              description={query.error.message}
              action={
                <Button variant="primary" onClick={() => void query.refetch()}>
                  Try again
                </Button>
              }
            />
          ) : query.data.length === 0 ? (
            <EmptyState
              icon={<Trash2 size={18} />}
              title="The trash is empty"
              description="Pages you move to the trash will show up here."
            />
          ) : (
            <ul className="surface divide-y divide-ink-100 overflow-hidden">
              {query.data.map((p) => (
                <li key={p.id} className="flex flex-wrap items-center gap-3 px-4 py-3">
                  <FileText size={16} className="shrink-0 text-ink-400" />
                  <div className="min-w-0 flex-1 basis-56">
                    <div className="flex items-center gap-2">
                      <Link
                        href={pageHref(key, p)}
                        className="truncate text-sm font-medium text-ink-900 hover:underline"
                      >
                        {p.title || "Untitled"}
                      </Link>
                      {p.status === "draft" && <Badge tone="amber">Draft</Badge>}
                      {p.has_children && <Badge>Has subpages</Badge>}
                    </div>
                    <div className="mt-0.5 text-xs text-ink-600">
                      Trashed {p.trashed_by_user ? `by ${p.trashed_by_user.display_name} ` : ""}
                      <RelativeTime iso={p.trashed_at} />
                    </div>
                  </div>
                  <div className="flex shrink-0 items-center gap-2">
                    {canEdit && (
                      <Button
                        size="sm"
                        onClick={() => restore.mutate(p)}
                        disabled={restore.isPending && restore.variables?.id === p.id}
                      >
                        <RotateCcw size={12} />
                        Restore
                      </Button>
                    )}
                    {isAdmin && (
                      <Button size="sm" variant="danger" onClick={() => setToPurge(p)}>
                        <Trash2 size={12} />
                        Delete permanently
                      </Button>
                    )}
                  </div>
                </li>
              ))}
            </ul>
          )}
        </div>
      </div>

      {toPurge && (
        <ConfirmDialog
          title="Delete permanently"
          tone="danger"
          confirmLabel="Delete permanently"
          busy={purge.isPending}
          onConfirm={() => purge.mutate(toPurge)}
          onCancel={() => setToPurge(null)}
          message={
            <>
              <strong>{toPurge.title || "This page"}</strong> and everything beneath it, including versions and
              attachments, will be deleted. This cannot be undone.
            </>
          }
        />
      )}
    </div>
  );
}
