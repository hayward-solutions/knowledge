"use client";

import { useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Copy,
  FolderInput,
  History,
  MoreHorizontal,
  Paperclip,
  Pencil,
  RotateCcw,
  Star,
  Trash2,
} from "lucide-react";
import {
  purgePage,
  restorePage,
  starPage,
  unstarPage,
  type PageDetail,
} from "@/lib/api";
import { cn } from "@/lib/cn";
import { Button } from "@/components/ui/Button";
import { ConfirmDialog } from "@/components/ui/ConfirmDialog";
import { Menu, MenuDivider, MenuItem } from "@/components/ui/Dropdown";
import { Tooltip } from "@/components/ui/Tooltip";
import { useToast } from "@/components/ui/Toast";
import { WatchToggle } from "@/components/notifications/WatchToggle";
import { useSpace } from "@/components/spaces/SpaceContext";
import { ConfirmTrash } from "./ConfirmTrash";
import { CopyDialog } from "./CopyDialog";
import { ExportMenu } from "./ExportMenu";
import { MenuLink } from "./MenuLink";
import { MoveDialog } from "./MoveDialog";
import {
  errorMessage,
  pageAttachmentsHref,
  pageEditHref,
  pageHistoryHref,
} from "./utils";

type Dialog = "move" | "copy" | "trash" | "purge" | null;

export function PageActions({ page }: { page: PageDetail }) {
  const router = useRouter();
  const qc = useQueryClient();
  const toast = useToast();
  const { space, canEdit, isAdmin, isEditor, refresh } = useSpace();
  const [dialog, setDialog] = useState<Dialog>(null);
  const trashed = page.status === "trashed";
  const isHome = page.id === space.home_page_id;
  const key = space.key;

  const star = useMutation({
    mutationFn: (next: boolean) => (next ? starPage(page.id) : unstarPage(page.id)),
    onMutate: async (next) => {
      await qc.cancelQueries({ queryKey: ["page", page.id] });
      const prev = qc.getQueryData<PageDetail>(["page", page.id]);
      if (prev) qc.setQueryData<PageDetail>(["page", page.id], { ...prev, starred: next });
      return { prev };
    },
    onError: (err, _next, ctx) => {
      if (ctx?.prev) qc.setQueryData(["page", page.id], ctx.prev);
      toast.error(errorMessage(err, "Could not update star"));
    },
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: ["dashboard"] });
    },
  });

  const restore = useMutation({
    mutationFn: () => restorePage(page.id),
    onSuccess: async () => {
      toast.success("Page restored");
      await Promise.all([
        refresh(),
        qc.invalidateQueries({ queryKey: ["page"] }),
        qc.invalidateQueries({ queryKey: ["trash", key] }),
        qc.invalidateQueries({ queryKey: ["dashboard"] }),
      ]);
    },
    onError: (err) => toast.error(errorMessage(err, "Could not restore page")),
  });

  const purge = useMutation({
    mutationFn: () => purgePage(page.id),
    onSuccess: async () => {
      toast.success("Page permanently deleted");
      setDialog(null);
      await Promise.all([
        refresh(),
        qc.invalidateQueries({ queryKey: ["trash", key] }),
        qc.invalidateQueries({ queryKey: ["dashboard"] }),
      ]);
      qc.removeQueries({ queryKey: ["page", page.id] });
      router.push(`/spaces/${key}/trash`);
    },
    onError: (err) => {
      setDialog(null);
      toast.error(errorMessage(err, "Could not delete page"));
    },
  });

  return (
    <div className="flex flex-wrap items-center gap-2 print:hidden">
      {trashed && canEdit && (
        <Button variant="secondary" onClick={() => restore.mutate()} disabled={restore.isPending}>
          <RotateCcw size={14} />
          Restore
        </Button>
      )}
      {trashed && isAdmin && (
        <Button variant="danger" onClick={() => setDialog("purge")}>
          <Trash2 size={14} />
          Delete permanently
        </Button>
      )}
      {isEditor &&
        (canEdit && !trashed ? (
          <Link href={pageEditHref(key, page.id)} className="btn btn-primary">
            <Pencil size={14} />
            Edit
          </Link>
        ) : (
          <Tooltip content={trashed ? "Restore this page to edit it" : "This space is read-only"}>
            <Button variant="primary" disabled>
              <Pencil size={14} />
              Edit
            </Button>
          </Tooltip>
        ))}

      <Tooltip content={page.starred ? "Remove star" : "Star this page"}>
        <Button
          variant="secondary"
          aria-pressed={page.starred}
          aria-label={page.starred ? "Remove star" : "Star this page"}
          onClick={() => star.mutate(!page.starred)}
          className="px-2.5"
        >
          <Star
            size={14}
            className={cn(page.starred && "fill-warning-500 text-warning-500")}
          />
        </Button>
      </Tooltip>

      <WatchToggle type="page" id={page.id} initial={page.watching} />

      <Menu
        align="right"
        width={220}
        label="More actions"
        trigger={(open) => (
          <span
            className={cn(
              "btn btn-secondary px-2.5",
              open && "bg-ink-100"
            )}
          >
            <MoreHorizontal size={16} />
          </span>
        )}
      >
        {(close) => (
          <>
            {canEdit && !trashed && !isHome && (
              <MenuItem
                onClick={() => {
                  close();
                  setDialog("move");
                }}
              >
                <FolderInput size={14} className="text-ink-500" />
                Move
              </MenuItem>
            )}
            {canEdit && !trashed && (
              <MenuItem
                onClick={() => {
                  close();
                  setDialog("copy");
                }}
              >
                <Copy size={14} className="text-ink-500" />
                Copy
              </MenuItem>
            )}
            <MenuLink href={pageAttachmentsHref(key, page.id)} onClick={close}>
              <Paperclip size={14} className="text-ink-500" />
              Attachments
            </MenuLink>
            <MenuLink href={pageHistoryHref(key, page.id)} onClick={close}>
              <History size={14} className="text-ink-500" />
              History
            </MenuLink>
            <MenuDivider />
            <ExportMenu page={page} onSelect={close} />
            {canEdit && !trashed && (
              <>
                <MenuDivider />
                <MenuItem
                  tone="danger"
                  disabled={isHome}
                  onClick={() => {
                    close();
                    setDialog("trash");
                  }}
                >
                  <Trash2 size={14} />
                  {isHome ? "Home page cannot be trashed" : "Move to trash"}
                </MenuItem>
              </>
            )}
          </>
        )}
      </Menu>

      {dialog === "move" && <MoveDialog pageId={page.id} onClose={() => setDialog(null)} />}
      {dialog === "copy" && <CopyDialog pageId={page.id} onClose={() => setDialog(null)} />}
      {dialog === "trash" && <ConfirmTrash pageId={page.id} onClose={() => setDialog(null)} />}
      {dialog === "purge" && (
        <ConfirmDialog
          title="Delete permanently"
          tone="danger"
          confirmLabel="Delete permanently"
          busy={purge.isPending}
          onConfirm={() => purge.mutate()}
          onCancel={() => setDialog(null)}
          message={
            <>
              <strong>{page.title || "This page"}</strong> and everything beneath it, including
              versions and attachments, will be deleted. This cannot be undone.
            </>
          }
        />
      )}
    </div>
  );
}
