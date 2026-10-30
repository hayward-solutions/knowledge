import type { PageSummary } from "@/lib/api";

export interface PageOption {
  id: string;
  title: string;
  depth: number;
}

export function flattenPages(pages: PageSummary[]): PageOption[] {
  const live = pages.filter((p) => p.status !== "trashed");
  const ids = new Set(live.map((p) => p.id));
  const byParent = new Map<string | null, PageSummary[]>();
  for (const p of live) {
    const parent = p.parent_id && ids.has(p.parent_id) ? p.parent_id : null;
    const list = byParent.get(parent) ?? [];
    list.push(p);
    byParent.set(parent, list);
  }
  const out: PageOption[] = [];
  const seen = new Set<string>();
  const walk = (parent: string | null, depth: number) => {
    const children = (byParent.get(parent) ?? []).slice().sort((a, b) => a.position - b.position);
    for (const c of children) {
      if (seen.has(c.id)) continue;
      seen.add(c.id);
      out.push({ id: c.id, title: c.title || "Untitled", depth });
      walk(c.id, depth + 1);
    }
  };
  walk(null, 0);
  return out;
}

export function indentLabel(o: PageOption): string {
  return `${" ".repeat(Math.min(o.depth, 8))}${o.title}`;
}
