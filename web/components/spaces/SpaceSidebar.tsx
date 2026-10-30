"use client";

import { useState } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { Activity, Archive, Plus, Settings, Trash2 } from "lucide-react";
import { cn } from "@/lib/cn";
import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { WatchToggle } from "@/components/notifications/WatchToggle";
import { ConfirmTrash } from "@/components/pages/ConfirmTrash";
import { CopyDialog } from "@/components/pages/CopyDialog";
import { MoveDialog } from "@/components/pages/MoveDialog";
import { NewPageDialog } from "@/components/pages/NewPageDialog";
import { BodyPortal } from "./BodyPortal";
import { PageTree, type TreeAction } from "./PageTree";
import { SpaceIcon } from "./SpaceIcon";
import { useSpace } from "./SpaceContext";
import { pageIdFromPath } from "./tree";

type DialogState =
  | { kind: "create"; parentId: string | null }
  | { kind: "move" | "copy" | "trash"; id: string }
  | null;

function FooterLink({
  href,
  icon,
  active,
  children,
}: {
  href: string;
  icon: React.ReactNode;
  active: boolean;
  children: React.ReactNode;
}) {
  return (
    <Link
      href={href}
      aria-current={active ? "page" : undefined}
      className={cn(
        "flex h-8 items-center gap-2 rounded-md px-2 text-sm font-medium transition",
        active ? "bg-brand-50 text-brand-700" : "text-ink-700 hover:bg-ink-100 hover:text-ink-900"
      )}
    >
      <span className="flex h-4 w-4 shrink-0 items-center justify-center">{icon}</span>
      <span className="truncate">{children}</span>
    </Link>
  );
}

export function SpaceSidebar() {
  const { space, canEdit, isEditor, isAdmin, archived } = useSpace();
  const pathname = usePathname();
  const [dialog, setDialog] = useState<DialogState>(null);
  const base = `/spaces/${space.key}`;
  const currentId =
    pageIdFromPath(pathname) ?? (pathname === base || pathname === `${base}/` ? space.home_page_id : null);

  function onAction(action: TreeAction, id: string) {
    if (action === "create-child") setDialog({ kind: "create", parentId: id });
    else setDialog({ kind: action, id });
  }

  return (
    <div className="flex min-h-full flex-col overflow-x-clip">
      <Link
        href={base}
        className="mb-2 flex items-center gap-2 rounded-md px-1 py-1 outline-none hover:bg-ink-100 focus-visible:ring-2 focus-visible:ring-brand-400"
      >
        <SpaceIcon space={space} size={28} />
        <span className="min-w-0 flex-1">
          <span className="block truncate text-sm font-semibold text-ink-900">{space.name}</span>
          <span className="block text-[11px] font-semibold uppercase tracking-wide text-ink-500">
            {space.key}
          </span>
        </span>
        {archived && (
          <Badge tone="amber" title="This space is archived">
            <Archive size={10} />
            Archived
          </Badge>
        )}
      </Link>

      <div className="mb-3 flex items-center gap-2">
        {canEdit && (
          <Button
            variant="secondary"
            className="min-w-0 flex-1 justify-start"
            onClick={() => setDialog({ kind: "create", parentId: currentId })}
          >
            <Plus size={14} />
            Create page
          </Button>
        )}
        <WatchToggle type="space" id={space.id} size="sm" className={canEdit ? "shrink-0" : "w-full"} />
      </div>

      <div className="mb-1 px-1 text-[11px] font-semibold uppercase tracking-wide text-ink-500">Pages</div>
      <div className="min-h-0 flex-1">
        <PageTree currentId={currentId} onAction={onAction} />
      </div>

      <nav
        aria-label="Space"
        className="sticky bottom-0 -mx-2 mt-3 space-y-0.5 border-t border-ink-200 bg-ink-0 px-2 pb-1 pt-2"
      >
        <FooterLink
          href={`${base}/activity`}
          icon={<Activity size={15} />}
          active={pathname.startsWith(`${base}/activity`)}
        >
          Activity
        </FooterLink>
        {isEditor && (
          <FooterLink
            href={`${base}/trash`}
            icon={<Trash2 size={15} />}
            active={pathname.startsWith(`${base}/trash`)}
          >
            Trash
          </FooterLink>
        )}
        {isAdmin && (
          <FooterLink
            href={`${base}/settings/general`}
            icon={<Settings size={15} />}
            active={pathname.startsWith(`${base}/settings`)}
          >
            Space settings
          </FooterLink>
        )}
      </nav>

      <BodyPortal>
        {dialog?.kind === "create" && (
          <NewPageDialog defaultParentId={dialog.parentId} onClose={() => setDialog(null)} />
        )}
        {dialog?.kind === "move" && <MoveDialog pageId={dialog.id} onClose={() => setDialog(null)} />}
        {dialog?.kind === "copy" && <CopyDialog pageId={dialog.id} onClose={() => setDialog(null)} />}
        {dialog?.kind === "trash" && <ConfirmTrash pageId={dialog.id} onClose={() => setDialog(null)} />}
      </BodyPortal>
    </div>
  );
}
