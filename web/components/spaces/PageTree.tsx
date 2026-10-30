"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import {
  DndContext,
  DragOverlay,
  MouseSensor,
  TouchSensor,
  pointerWithin,
  useSensor,
  useSensors,
  type DragEndEvent,
  type DragMoveEvent,
  type DragStartEvent,
} from "@dnd-kit/core";
import { Copy, ExternalLink, FolderInput, Link2, Pencil, Plus, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/Button";
import { useToast } from "@/components/ui/Toast";
import { pageEditHref, pageHref, absoluteUrl } from "@/components/pages/utils";
import { ListSkeleton } from "@/components/pages/Skeletons";
import { BodyPortal } from "./BodyPortal";
import { useSpace } from "./SpaceContext";
import { TreeContextMenu, type ContextMenuItem } from "./TreeContextMenu";
import { TreeNode, type DropWhere } from "./TreeNode";
import { useMovePage } from "./useMovePage";
import { useTreeState } from "./useTreeState";
import {
  ancestorIds,
  applyMove,
  buildTree,
  descendantIds,
  flattenVisible,
  sameOrder,
  siblingsOf,
  type MoveTarget,
} from "./tree";

export type TreeAction = "create-child" | "move" | "copy" | "trash";

interface DropState {
  overId: string;
  where: DropWhere;
}

function pointerY(e: Event | null | undefined): number | null {
  if (!e) return null;
  if ("clientY" in e && typeof (e as MouseEvent).clientY === "number") return (e as MouseEvent).clientY;
  const t = (e as TouchEvent).touches?.[0] ?? (e as TouchEvent).changedTouches?.[0];
  return t ? t.clientY : null;
}

export function PageTree({
  currentId,
  onAction,
}: {
  currentId: string | null;
  onAction: (action: TreeAction, id: string) => void;
}) {
  const router = useRouter();
  const toast = useToast();
  const move = useMovePage();
  const { space, tree, treeLoading, treeError, canEdit, refresh } = useSpace();

  const nodes = useMemo(() => buildTree(tree), [tree]);
  const defaultExpanded = useMemo(
    () => nodes.filter((n) => n.children.length > 0).map((n) => n.page.id),
    [nodes]
  );
  const state = useTreeState(space.key, defaultExpanded);
  const visible = useMemo(() => flattenVisible(nodes, state.expanded), [nodes, state.expanded]);
  const byId = useMemo(() => new Map(tree.map((p) => [p.id, p])), [tree]);

  const [focusedId, setFocusedId] = useState<string | null>(null);
  const [menu, setMenu] = useState<{ id: string; x: number; y: number } | null>(null);
  const [activeId, setActiveId] = useState<string | null>(null);
  const [drop, setDrop] = useState<DropState | null>(null);
  const blocked = useRef<Set<string>>(new Set());
  const refs = useRef(new Map<string, HTMLElement>());
  const lastExpandedFor = useRef<string | null>(null);
  const expandRef = useRef(state.expand);
  expandRef.current = state.expand;

  useEffect(() => {
    if (!currentId || tree.length === 0) return;
    if (lastExpandedFor.current === currentId) return;
    lastExpandedFor.current = currentId;
    const ancestors = ancestorIds(tree, currentId);
    if (ancestors.length > 0) expandRef.current(ancestors);
  }, [currentId, tree]);

  const sensors = useSensors(
    useSensor(MouseSensor, { activationConstraint: { distance: 6 } }),
    useSensor(TouchSensor, { activationConstraint: { delay: 250, tolerance: 8 } })
  );

  const registerRef = useCallback((id: string, el: HTMLElement | null) => {
    if (el) refs.current.set(id, el);
    else refs.current.delete(id);
  }, []);

  const tabStopId = useMemo(() => {
    if (focusedId && visible.some((v) => v.page.id === focusedId)) return focusedId;
    if (currentId && visible.some((v) => v.page.id === currentId)) return currentId;
    return visible[0]?.page.id ?? null;
  }, [focusedId, currentId, visible]);

  function focusItem(id: string | undefined) {
    if (!id) return;
    setFocusedId(id);
    refs.current.get(id)?.focus();
  }

  function hrefFor(id: string): string {
    const p = byId.get(id);
    if (!p) return `/spaces/${space.key}`;
    if (p.status === "draft" && canEdit) return pageEditHref(space.key, id);
    return pageHref(space.key, p);
  }

  function openMenuFor(id: string, x: number, y: number) {
    setFocusedId(id);
    setMenu({ id, x, y });
  }

  function onFocusCapture(e: React.FocusEvent<HTMLDivElement>) {
    const id = (e.target as HTMLElement).closest<HTMLElement>("[data-tree-id]")?.dataset.treeId;
    if (id) setFocusedId(id);
  }

  function onKeyDown(e: React.KeyboardEvent<HTMLDivElement>) {
    if (e.target instanceof HTMLElement && e.target.closest('[role="menu"]')) return;
    const el = (e.target as HTMLElement).closest<HTMLElement>("[data-tree-id]");
    const id = el?.dataset.treeId;
    if (!id) return;
    const idx = visible.findIndex((v) => v.page.id === id);
    if (idx < 0) return;
    const node = visible[idx];
    switch (e.key) {
      case "ArrowDown":
        e.preventDefault();
        focusItem(visible[Math.min(idx + 1, visible.length - 1)]?.page.id);
        break;
      case "ArrowUp":
        e.preventDefault();
        focusItem(visible[Math.max(idx - 1, 0)]?.page.id);
        break;
      case "Home":
        e.preventDefault();
        focusItem(visible[0]?.page.id);
        break;
      case "End":
        e.preventDefault();
        focusItem(visible[visible.length - 1]?.page.id);
        break;
      case "ArrowRight":
        e.preventDefault();
        if (node.hasChildren && !state.isExpanded(id)) state.expand([id]);
        else if (node.hasChildren) focusItem(visible[idx + 1]?.page.id);
        break;
      case "ArrowLeft":
        e.preventDefault();
        if (node.hasChildren && state.isExpanded(id)) state.collapse(id);
        else if (node.parentId) focusItem(node.parentId);
        break;
      case "Enter":
      case " ":
        if (e.target !== el) return;
        e.preventDefault();
        router.push(hrefFor(id));
        break;
      case "ContextMenu":
      case "F10":
        if (e.key === "F10" && !e.shiftKey) return;
        if (!canEdit) return;
        e.preventDefault();
        {
          const r = el.getBoundingClientRect();
          openMenuFor(id, r.left + 24, r.bottom);
        }
        break;
      default:
        break;
    }
  }

  function buildMenuItems(): ContextMenuItem[] {
    if (!menu) return [];
    const id = menu.id;
    const page = byId.get(id);
    if (!page) return [];
    const isHome = id === space.home_page_id;
    const items: ContextMenuItem[] = [
      {
        key: "open",
        label: "Open",
        icon: <ExternalLink size={14} className="text-ink-500" />,
        onSelect: () => router.push(page.status === "draft" ? hrefFor(id) : pageHref(space.key, page)),
      },
    ];
    if (canEdit) {
      items.push(
        {
          key: "edit",
          label: "Edit",
          icon: <Pencil size={14} className="text-ink-500" />,
          onSelect: () => router.push(pageEditHref(space.key, id)),
        },
        {
          key: "child",
          label: "Create child",
          icon: <Plus size={14} className="text-ink-500" />,
          onSelect: () => onAction("create-child", id),
        }
      );
      if (!isHome) {
        items.push({
          key: "move",
          label: "Move",
          icon: <FolderInput size={14} className="text-ink-500" />,
          onSelect: () => onAction("move", id),
        });
      }
      items.push({
        key: "copy",
        label: "Copy",
        icon: <Copy size={14} className="text-ink-500" />,
        onSelect: () => onAction("copy", id),
      });
    }
    items.push({
      key: "link",
      label: "Copy link",
      icon: <Link2 size={14} className="text-ink-500" />,
      separatorBefore: true,
      onSelect: () => {
        navigator.clipboard
          ?.writeText(absoluteUrl(pageHref(space.key, page)))
          .then(() => toast.success("Link copied"))
          .catch(() => toast.error("Could not copy link"));
      },
    });
    if (canEdit && !isHome) {
      items.push({
        key: "trash",
        label: "Move to trash",
        tone: "danger",
        icon: <Trash2 size={14} />,
        separatorBefore: true,
        onSelect: () => onAction("trash", id),
      });
    }
    return items;
  }

  const closeMenu = useCallback(() => {
    setMenu((m) => {
      if (m) refs.current.get(m.id)?.focus();
      return null;
    });
  }, []);

  function onDragStart(e: DragStartEvent) {
    const id = String(e.active.id);
    setActiveId(id);
    const set = descendantIds(tree, id);
    set.add(id);
    blocked.current = set;
  }

  function computeTarget(overId: string, where: DropWhere, dragId: string): MoveTarget | null {
    const over = byId.get(overId);
    if (!over) return null;
    if (where === "onto") return { parentId: overId, afterId: null };
    const overFlat = visible.find((v) => v.page.id === overId);
    if (where === "after" && overFlat?.hasChildren && state.isExpanded(overId)) {
      return { parentId: overId, afterId: null, position: 0 };
    }
    const parentId = over.parent_id ?? null;
    if (parentId === null) return { parentId: overId, afterId: null };
    const sibs = siblingsOf(tree, parentId, dragId);
    const i = sibs.findIndex((s) => s.id === overId);
    if (where === "after") return { parentId, afterId: overId };
    if (i <= 0) return { parentId, afterId: null, position: 0 };
    return { parentId, afterId: sibs[i - 1].id };
  }

  function onDragMove(e: DragMoveEvent) {
    const over = e.over;
    if (!over) {
      setDrop(null);
      return;
    }
    const overId = String(over.id);
    if (blocked.current.has(overId)) {
      setDrop(null);
      return;
    }
    const startY = pointerY(e.activatorEvent);
    if (startY === null) return;
    const y = startY + e.delta.y;
    const rel = (y - over.rect.top) / Math.max(over.rect.height, 1);
    const overPage = byId.get(overId);
    const isRoot = !overPage?.parent_id;
    const where: DropWhere = isRoot ? "onto" : rel < 0.28 ? "before" : rel > 0.72 ? "after" : "onto";
    setDrop((cur) => (cur && cur.overId === overId && cur.where === where ? cur : { overId, where }));
  }

  async function onDragEnd(e: DragEndEvent) {
    const dragId = String(e.active.id);
    const current = drop;
    setActiveId(null);
    setDrop(null);
    if (!current) return;
    const target = computeTarget(current.overId, current.where, dragId);
    if (!target || target.afterId === dragId) return;
    const moving = byId.get(dragId);
    if (!moving) return;
    const after = applyMove(tree, dragId, target);
    if ((moving.parent_id ?? null) === target.parentId && sameOrder(tree, after, target.parentId)) return;
    const ok = await move(dragId, target);
    if (ok && target.parentId) state.expand([target.parentId]);
  }

  function onDragCancel() {
    setActiveId(null);
    setDrop(null);
  }

  const activePage = activeId ? byId.get(activeId) : undefined;

  if (treeLoading) return <ListSkeleton rows={6} />;

  if (treeError) {
    return (
      <div className="space-y-2 px-1 text-sm text-ink-600">
        <p>Could not load the page tree.</p>
        <Button size="sm" onClick={() => void refresh()}>
          Retry
        </Button>
      </div>
    );
  }

  if (visible.length === 0) {
    return <p className="px-2 text-sm text-ink-600">This space has no pages yet.</p>;
  }

  const sizes = new Map<string | null, number>();
  const positions = new Map<string, number>();
  for (const v of visible) {
    const n = (sizes.get(v.parentId) ?? 0) + 1;
    sizes.set(v.parentId, n);
    positions.set(v.page.id, n);
  }
  const totals = new Map<string | null, number>();
  for (const p of tree) totals.set(p.parent_id ?? null, (totals.get(p.parent_id ?? null) ?? 0) + 1);

  return (
    <>
      <DndContext
        sensors={sensors}
        collisionDetection={pointerWithin}
        onDragStart={onDragStart}
        onDragMove={onDragMove}
        onDragEnd={onDragEnd}
        onDragCancel={onDragCancel}
        accessibility={{
          announcements: {
            onDragStart: ({ active }) => `Picked up ${byId.get(String(active.id))?.title ?? "page"}`,
            onDragOver: ({ over }) => (over ? `Over ${byId.get(String(over.id))?.title ?? "page"}` : undefined),
            onDragEnd: ({ active }) => `Dropped ${byId.get(String(active.id))?.title ?? "page"}`,
            onDragCancel: ({ active }) => `Cancelled moving ${byId.get(String(active.id))?.title ?? "page"}`,
          },
        }}
      >
        <div
          role="tree"
          aria-label="Page tree"
          onKeyDown={onKeyDown}
          onFocus={onFocusCapture}
          className="space-y-px"
        >
          {visible.map((v) => (
            <TreeNode
              key={v.page.id}
              node={v}
              href={hrefFor(v.page.id)}
              current={v.page.id === currentId}
              expanded={state.isExpanded(v.page.id)}
              focusable={v.page.id === tabStopId}
              dropWhere={drop && drop.overId === v.page.id ? drop.where : null}
              draggable={canEdit && v.page.id !== space.home_page_id}
              canEdit={canEdit}
              setSize={totals.get(v.parentId) ?? 1}
              posInSet={positions.get(v.page.id) ?? 1}
              registerRef={registerRef}
              onToggle={state.toggle}
              onMenu={openMenuFor}
              onCreateChild={(id) => onAction("create-child", id)}
            />
          ))}
        </div>
        <DragOverlay dropAnimation={null}>
          {activePage ? (
            <div className="pointer-events-none max-w-[220px] truncate rounded-md border border-brand-300 bg-ink-0 px-3 py-1.5 text-sm font-medium text-ink-900 shadow-overlay">
              {activePage.title || "Untitled"}
            </div>
          ) : null}
        </DragOverlay>
      </DndContext>
      {menu && (
        <BodyPortal>
          <TreeContextMenu
            x={menu.x}
            y={menu.y}
            label={`Actions for ${byId.get(menu.id)?.title ?? "page"}`}
            items={buildMenuItems()}
            onClose={closeMenu}
          />
        </BodyPortal>
      )}
    </>
  );
}
