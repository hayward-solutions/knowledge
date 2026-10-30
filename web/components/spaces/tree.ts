import type { PageSummary } from "@/lib/api";

export interface TreeNodeData {
  page: PageSummary;
  children: TreeNodeData[];
}

export interface FlatNode {
  page: PageSummary;
  depth: number;
  parentId: string | null;
  hasChildren: boolean;
}

export function byPosition(a: PageSummary, b: PageSummary): number {
  return a.position - b.position || a.title.localeCompare(b.title);
}

export function buildTree(pages: PageSummary[]): TreeNodeData[] {
  const ids = new Set(pages.map((p) => p.id));
  const byParent = new Map<string | null, PageSummary[]>();
  for (const p of pages) {
    const parent = p.parent_id && ids.has(p.parent_id) ? p.parent_id : null;
    const list = byParent.get(parent);
    if (list) list.push(p);
    else byParent.set(parent, [p]);
  }
  const make = (parent: string | null, seen: Set<string>): TreeNodeData[] =>
    (byParent.get(parent) ?? []).sort(byPosition).map((page) => {
      if (seen.has(page.id)) return { page, children: [] };
      const next = new Set(seen);
      next.add(page.id);
      return { page, children: make(page.id, next) };
    });
  return make(null, new Set());
}

export function flattenAll(nodes: TreeNodeData[], depth = 0, parentId: string | null = null): FlatNode[] {
  const out: FlatNode[] = [];
  for (const n of nodes) {
    out.push({ page: n.page, depth, parentId, hasChildren: n.children.length > 0 });
    out.push(...flattenAll(n.children, depth + 1, n.page.id));
  }
  return out;
}

export function flattenVisible(
  nodes: TreeNodeData[],
  expanded: Set<string>,
  depth = 0,
  parentId: string | null = null
): FlatNode[] {
  const out: FlatNode[] = [];
  for (const n of nodes) {
    out.push({ page: n.page, depth, parentId, hasChildren: n.children.length > 0 });
    if (n.children.length > 0 && expanded.has(n.page.id)) {
      out.push(...flattenVisible(n.children, expanded, depth + 1, n.page.id));
    }
  }
  return out;
}

export function descendantIds(pages: PageSummary[], id: string): Set<string> {
  const children = new Map<string, string[]>();
  for (const p of pages) {
    if (!p.parent_id) continue;
    const list = children.get(p.parent_id);
    if (list) list.push(p.id);
    else children.set(p.parent_id, [p.id]);
  }
  const out = new Set<string>();
  const stack = [...(children.get(id) ?? [])];
  while (stack.length > 0) {
    const next = stack.pop() as string;
    if (out.has(next)) continue;
    out.add(next);
    stack.push(...(children.get(next) ?? []));
  }
  return out;
}

export function ancestorIds(pages: PageSummary[], id: string): string[] {
  const byId = new Map(pages.map((p) => [p.id, p]));
  const out: string[] = [];
  const seen = new Set<string>([id]);
  let cur = byId.get(id)?.parent_id ?? null;
  while (cur && !seen.has(cur)) {
    out.push(cur);
    seen.add(cur);
    cur = byId.get(cur)?.parent_id ?? null;
  }
  return out;
}

export function siblingsOf(pages: PageSummary[], parentId: string | null, excludeId?: string): PageSummary[] {
  return pages
    .filter((p) => (p.parent_id ?? null) === parentId && p.id !== excludeId)
    .sort(byPosition);
}

export interface MoveTarget {
  parentId: string | null;
  afterId: string | null;
  position?: number;
}

export function applyMove(pages: PageSummary[], id: string, target: MoveTarget): PageSummary[] {
  const moving = pages.find((p) => p.id === id);
  if (!moving) return pages;
  const oldParent = moving.parent_id ?? null;
  const siblings = siblingsOf(pages, target.parentId, id);
  let idx: number;
  if (target.afterId) {
    const i = siblings.findIndex((s) => s.id === target.afterId);
    idx = i < 0 ? siblings.length : i + 1;
  } else if (target.position !== undefined) {
    idx = Math.min(Math.max(target.position, 0), siblings.length);
  } else {
    idx = siblings.length;
  }
  const placed = [
    ...siblings.slice(0, idx),
    { ...moving, parent_id: target.parentId },
    ...siblings.slice(idx),
  ];
  const positions = new Map(placed.map((p, i) => [p.id, i]));
  if (oldParent !== target.parentId) {
    siblingsOf(pages, oldParent, id).forEach((p, i) => positions.set(p.id, i));
  }
  const next = pages.map((p) => {
    const pos = positions.get(p.id);
    if (p.id === id) return { ...p, parent_id: target.parentId, position: pos ?? p.position };
    return pos === undefined ? p : { ...p, position: pos };
  });
  const parents = new Set(next.map((p) => p.parent_id).filter((x): x is string => !!x));
  return next.map((p) => (p.has_children === parents.has(p.id) ? p : { ...p, has_children: parents.has(p.id) }));
}

export function sameOrder(a: PageSummary[], b: PageSummary[], parentId: string | null): boolean {
  const ids = (list: PageSummary[]) => siblingsOf(list, parentId).map((p) => p.id).join(",");
  return ids(a) === ids(b);
}

export function pageIdFromPath(pathname: string): string | null {
  const m = pathname.match(/^\/spaces\/[^/]+\/pages\/([^/]+)/);
  return m ? decodeURIComponent(m[1]) : null;
}
