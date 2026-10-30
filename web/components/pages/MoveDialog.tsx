"use client";

import { useMemo, useState } from "react";
import { FileText } from "lucide-react";
import { cn } from "@/lib/cn";
import { Button } from "@/components/ui/Button";
import { Field } from "@/components/ui/Field";
import { Input } from "@/components/ui/Input";
import { Modal } from "@/components/ui/Modal";
import { Select } from "@/components/ui/Select";
import { useToast } from "@/components/ui/Toast";
import { useSpace } from "@/components/spaces/SpaceContext";
import { useMovePage } from "@/components/spaces/useMovePage";
import { buildTree, descendantIds, flattenAll, siblingsOf, type MoveTarget } from "@/components/spaces/tree";

export function MoveDialog({
  pageId,
  onClose,
  onMoved,
}: {
  pageId: string;
  onClose: () => void;
  onMoved?: () => void;
}) {
  const toast = useToast();
  const move = useMovePage();
  const { tree } = useSpace();
  const page = tree.find((p) => p.id === pageId);
  const initialParent = page?.parent_id ?? null;
  const [parentId, setParentId] = useState<string | null>(initialParent);
  const [filter, setFilter] = useState("");
  const [placement, setPlacement] = useState<string>(() => {
    const sibs = siblingsOf(tree, initialParent, pageId);
    const mine = tree.find((p) => p.id === pageId);
    if (!mine) return "end";
    const before = siblingsOf(tree, initialParent).findIndex((p) => p.id === pageId);
    if (before <= 0) return sibs.length === 0 ? "end" : "start";
    return `after:${siblingsOf(tree, initialParent)[before - 1].id}`;
  });
  const [busy, setBusy] = useState(false);

  const excluded = useMemo(() => {
    const set = descendantIds(tree, pageId);
    set.add(pageId);
    return set;
  }, [tree, pageId]);

  const candidates = useMemo(() => {
    const all = flattenAll(buildTree(tree)).filter((n) => !excluded.has(n.page.id));
    const q = filter.trim().toLowerCase();
    if (!q) return all;
    return all
      .filter((n) => n.page.title.toLowerCase().includes(q))
      .map((n) => ({ ...n, depth: 0 }));
  }, [tree, excluded, filter]);

  const siblings = useMemo(() => siblingsOf(tree, parentId, pageId), [tree, parentId, pageId]);

  function chooseParent(id: string) {
    setParentId(id);
    setPlacement("end");
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    if (!parentId) return;
    let target: MoveTarget;
    if (placement === "start") target = { parentId, afterId: null, position: 0 };
    else if (placement.startsWith("after:")) target = { parentId, afterId: placement.slice(6) };
    else target = { parentId, afterId: null };
    setBusy(true);
    const ok = await move(pageId, target);
    setBusy(false);
    if (ok) {
      toast.success("Page moved");
      onMoved?.();
      onClose();
    }
  }

  return (
    <Modal
      title={`Move "${page?.title ?? "page"}"`}
      onClose={onClose}
      width={520}
      footer={
        <>
          <Button variant="ghost" onClick={onClose} disabled={busy}>
            Cancel
          </Button>
          <Button variant="primary" type="submit" form="move-page-form" disabled={busy || !parentId}>
            {busy ? "Moving..." : "Move"}
          </Button>
        </>
      }
    >
      <form id="move-page-form" onSubmit={submit} className="space-y-4">
        <Field label="Find a new parent page">
          <Input
            data-autofocus
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            placeholder="Filter pages"
          />
        </Field>
        <div
          role="radiogroup"
          aria-label="New parent page"
          className="max-h-64 overflow-y-auto rounded-md border border-ink-200"
        >
          {candidates.length === 0 && (
            <div className="px-3 py-4 text-sm text-ink-600">No matching pages.</div>
          )}
          {candidates.map((n) => (
            <label
              key={n.page.id}
              className={cn(
                "flex cursor-pointer items-center gap-2 py-1.5 pr-3 text-sm hover:bg-ink-50",
                parentId === n.page.id && "bg-brand-50 text-brand-700"
              )}
              style={{ paddingLeft: 12 + n.depth * 16 }}
            >
              <input
                type="radio"
                name="new-parent"
                checked={parentId === n.page.id}
                onChange={() => chooseParent(n.page.id)}
              />
              <FileText size={14} className="shrink-0 text-ink-500" />
              <span className="min-w-0 flex-1 truncate">{n.page.title || "Untitled"}</span>
            </label>
          ))}
        </div>
        <Field label="Position">
          <Select value={placement} onChange={(e) => setPlacement(e.target.value)} className="w-full">
            <option value="end">At the end</option>
            <option value="start">At the beginning</option>
            {siblings.map((s) => (
              <option key={s.id} value={`after:${s.id}`}>
                After {s.title || "Untitled"}
              </option>
            ))}
          </Select>
        </Field>
      </form>
    </Modal>
  );
}
