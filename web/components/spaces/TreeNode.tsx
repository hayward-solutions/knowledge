"use client";

import Link from "next/link";
import { useDraggable, useDroppable } from "@dnd-kit/core";
import { ChevronRight, FileText, MoreHorizontal, Plus } from "lucide-react";
import { cn } from "@/lib/cn";
import { Badge } from "@/components/ui/Badge";
import type { FlatNode } from "./tree";

export type DropWhere = "before" | "after" | "onto";

export interface TreeNodeProps {
  node: FlatNode;
  href: string;
  current: boolean;
  expanded: boolean;
  focusable: boolean;
  dropWhere: DropWhere | null;
  draggable: boolean;
  canEdit: boolean;
  setSize: number;
  posInSet: number;
  registerRef: (id: string, el: HTMLElement | null) => void;
  onToggle: (id: string) => void;
  onMenu: (id: string, x: number, y: number) => void;
  onCreateChild: (id: string) => void;
}

export function TreeNode({
  node,
  href,
  current,
  expanded,
  focusable,
  dropWhere,
  draggable,
  canEdit,
  setSize,
  posInSet,
  registerRef,
  onToggle,
  onMenu,
  onCreateChild,
}: TreeNodeProps) {
  const { page, depth, hasChildren } = node;
  const drag = useDraggable({ id: page.id, disabled: !draggable });
  const drop = useDroppable({ id: page.id });

  const setRefs = (el: HTMLDivElement | null) => {
    drag.setNodeRef(el);
    drop.setNodeRef(el);
    registerRef(page.id, el);
  };

  const title = page.title || "Untitled";

  return (
    <div
      ref={setRefs}
      role="treeitem"
      aria-level={depth + 1}
      aria-setsize={setSize}
      aria-posinset={posInSet}
      aria-expanded={hasChildren ? expanded : undefined}
      aria-selected={current}
      aria-label={title}
      tabIndex={focusable ? 0 : -1}
      data-tree-id={page.id}
      onContextMenu={(e) => {
        if (!canEdit) return;
        e.preventDefault();
        onMenu(page.id, e.clientX, e.clientY);
      }}
      {...(draggable ? drag.listeners : {})}
      className={cn(
        "group relative flex h-8 select-none items-center rounded-md pr-1 text-sm outline-none transition-colors",
        "focus-visible:ring-2 focus-visible:ring-brand-400",
        current ? "bg-brand-50 font-medium text-brand-700" : "text-ink-700 hover:bg-ink-100 hover:text-ink-900",
        drag.isDragging && "opacity-40",
        dropWhere === "onto" && "bg-brand-100 ring-1 ring-brand-500"
      )}
      style={{ paddingLeft: 2 + depth * 14 }}
    >
      {dropWhere === "before" && (
        <span
          aria-hidden
          className="pointer-events-none absolute -top-px right-1 h-0.5 rounded bg-brand-500"
          style={{ left: 2 + depth * 14 }}
        />
      )}
      {dropWhere === "after" && (
        <span
          aria-hidden
          className="pointer-events-none absolute -bottom-px right-1 h-0.5 rounded bg-brand-500"
          style={{ left: 2 + depth * 14 }}
        />
      )}
      <button
        type="button"
        tabIndex={-1}
        aria-label={hasChildren ? (expanded ? `Collapse ${title}` : `Expand ${title}`) : undefined}
        aria-hidden={hasChildren ? undefined : true}
        onClick={() => hasChildren && onToggle(page.id)}
        className={cn(
          "flex h-6 w-5 shrink-0 items-center justify-center rounded text-ink-500 hover:bg-ink-200",
          !hasChildren && "pointer-events-none invisible"
        )}
      >
        <ChevronRight size={14} className={cn("transition-transform", expanded && "rotate-90")} />
      </button>
      <Link
        href={href}
        tabIndex={-1}
        draggable={false}
        className="flex min-w-0 flex-1 items-center gap-1.5 py-1 outline-none"
      >
        <FileText size={14} className="shrink-0 text-ink-400" />
        <span className={cn("truncate", page.status === "draft" && "italic")}>{title}</span>
        {page.status === "draft" && (
          <Badge tone="amber" className="ml-1 shrink-0">
            Draft
          </Badge>
        )}
      </Link>
      {canEdit && (
        <span className="flex shrink-0 items-center opacity-0 transition-opacity focus-within:opacity-100 group-hover:opacity-100 max-md:opacity-100">
          <button
            type="button"
            tabIndex={-1}
            aria-label={`Create child page under ${title}`}
            onClick={() => onCreateChild(page.id)}
            className="flex h-6 w-6 items-center justify-center rounded text-ink-500 hover:bg-ink-200 hover:text-ink-900"
          >
            <Plus size={14} />
          </button>
          <button
            type="button"
            tabIndex={-1}
            aria-label={`Actions for ${title}`}
            aria-haspopup="menu"
            onClick={(e) => {
              const r = e.currentTarget.getBoundingClientRect();
              onMenu(page.id, r.left, r.bottom + 2);
            }}
            className="flex h-6 w-6 items-center justify-center rounded text-ink-500 hover:bg-ink-200 hover:text-ink-900"
          >
            <MoreHorizontal size={14} />
          </button>
        </span>
      )}
    </div>
  );
}
