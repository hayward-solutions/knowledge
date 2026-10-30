"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Plus, Tag, X } from "lucide-react";
import { ApiError } from "@/lib/api/client";
import { getPageLabels, setPageLabels } from "@/lib/api/labels";
import type { Label } from "@/lib/api/types";
import { useToast } from "@/components/ui/Toast";
import { LabelPicker } from "./LabelPicker";
import { MAX_LABELS_PER_PAGE } from "./normalize";

export interface PageLabelsProps {
  pageId: string;
  canEdit: boolean;
  initial?: Label[];
}

export function PageLabels({ pageId, canEdit, initial }: PageLabelsProps) {
  const params = useParams<{ key?: string | string[] }>();
  const rawKey = params?.key;
  const spaceKey = Array.isArray(rawKey) ? rawKey[0] : rawKey;
  const qc = useQueryClient();
  const toast = useToast();
  const [adding, setAdding] = useState(false);
  const [saving, setSaving] = useState(false);

  const { data } = useQuery({
    queryKey: ["page-labels", pageId],
    queryFn: () => getPageLabels(pageId),
    initialData: initial,
  });

  const labels = data ?? initial ?? [];
  const names = labels.map((l) => l.name);

  async function save(next: string[]) {
    setSaving(true);
    try {
      const saved = await setPageLabels(pageId, next);
      qc.setQueryData(["page-labels", pageId], saved);
      void qc.invalidateQueries({ queryKey: ["labels"] });
      void qc.invalidateQueries({ queryKey: ["space-labels"] });
      void qc.invalidateQueries({ queryKey: ["label-pages"] });
    } catch (e) {
      if (e instanceof ApiError && e.code === "page_trashed") {
        toast.error("This page is in the trash, so its labels can no longer be changed.");
      } else {
        toast.error(e instanceof Error ? e.message : "Could not update labels");
      }
    } finally {
      setSaving(false);
    }
  }

  function add(name: string) {
    if (saving || names.includes(name)) return;
    if (names.length >= MAX_LABELS_PER_PAGE) {
      toast.error(`A page can have at most ${MAX_LABELS_PER_PAGE} labels`);
      return;
    }
    void save([...names, name]);
  }

  function remove(name: string) {
    void save(names.filter((n) => n !== name));
  }

  if (labels.length === 0 && !canEdit) return null;

  return (
    <div aria-label="Labels" className="flex flex-wrap items-center gap-1.5">
      <Tag size={14} className="text-ink-500" aria-hidden />
      {labels.map((label) => {
        const chipClass =
          "inline-flex h-6 items-center rounded-full bg-ink-100 text-xs font-medium text-ink-700";
        return (
          <span key={label.id} className={chipClass}>
            {spaceKey ? (
              <Link
                href={`/spaces/${spaceKey}/labels/${encodeURIComponent(label.name)}`}
                className={
                  canEdit
                    ? "rounded-l-full py-0.5 pl-2.5 pr-1 hover:text-brand-700"
                    : "rounded-full px-2.5 py-0.5 hover:text-brand-700"
                }
              >
                {label.name}
              </Link>
            ) : (
              <span className={canEdit ? "py-0.5 pl-2.5 pr-1" : "px-2.5 py-0.5"}>{label.name}</span>
            )}
            {canEdit && (
              <button
                type="button"
                aria-label={`Remove label ${label.name}`}
                disabled={saving}
                onClick={() => remove(label.name)}
                className="mr-1 flex h-4 w-4 items-center justify-center rounded-full text-ink-500 hover:bg-ink-200 hover:text-ink-900 disabled:opacity-50"
              >
                <X size={11} />
              </button>
            )}
          </span>
        );
      })}
      {canEdit &&
        (adding ? (
          <div
            onBlur={(e) => {
              if (!e.currentTarget.contains(e.relatedTarget as Node | null)) setAdding(false);
            }}
          >
            <LabelPicker
              autoFocus
              selected={names}
              onSelect={add}
              onClose={() => setAdding(false)}
            />
          </div>
        ) : (
          <button
            type="button"
            onClick={() => setAdding(true)}
            className="inline-flex h-6 items-center gap-1 rounded-full border border-dashed border-ink-300 px-2.5 text-xs font-medium text-ink-600 hover:border-ink-400 hover:text-ink-900"
          >
            <Plus size={12} />
            Add label
          </button>
        ))}
    </div>
  );
}
