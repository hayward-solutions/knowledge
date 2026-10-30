"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { FileText, Pencil, Plus, Trash2 } from "lucide-react";
import {
  createGlobalTemplate,
  createSpaceTemplate,
  deleteTemplate,
  listSpaceTemplates,
  listTemplates,
  updateTemplate,
  type Template,
  type TemplateInput,
} from "@/lib/api";
import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { ConfirmDialog } from "@/components/ui/ConfirmDialog";
import { EmptyState } from "@/components/ui/EmptyState";
import { Spinner } from "@/components/ui/Spinner";
import { useToast } from "@/components/ui/Toast";
import { TemplateEditor } from "./TemplateEditor";
import { TemplatePreviewModal } from "./TemplatePreviewModal";

export type TemplateScope = { kind: "global" } | { kind: "space"; key: string };

function formatDate(iso: string): string {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? "" : d.toLocaleDateString();
}

export function TemplateManager({
  scope,
  canManage,
}: {
  scope: TemplateScope;
  canManage: boolean;
}) {
  const qc = useQueryClient();
  const toast = useToast();
  const [editing, setEditing] = useState<Template | "new" | null>(null);
  const [previewing, setPreviewing] = useState<Template | null>(null);
  const [deleting, setDeleting] = useState<Template | null>(null);
  const [formError, setFormError] = useState<string | null>(null);

  const queryKey =
    scope.kind === "global"
      ? (["templates", "global"] as const)
      : (["templates", "space", scope.key] as const);

  const list = useQuery({
    queryKey,
    queryFn: async () => {
      if (scope.kind === "space") return listSpaceTemplates(scope.key);
      const all = await listTemplates();
      return all.filter((t) => t.space_id === null);
    },
  });

  const refresh = () => qc.invalidateQueries({ queryKey: ["templates"] });

  const save = useMutation({
    mutationFn: async (input: TemplateInput) => {
      if (editing && editing !== "new") return updateTemplate(editing.id, input);
      return scope.kind === "global"
        ? createGlobalTemplate(input)
        : createSpaceTemplate(scope.key, input);
    },
    onSuccess: () => {
      toast.success(editing === "new" ? "Template created" : "Template saved");
      setEditing(null);
      setFormError(null);
      refresh();
    },
    onError: (e: Error) => setFormError(e.message),
  });

  const remove = useMutation({
    mutationFn: (t: Template) => deleteTemplate(t.id),
    onSuccess: () => {
      toast.success("Template deleted");
      setDeleting(null);
      refresh();
    },
    onError: (e: Error) => {
      toast.error(e.message);
      setDeleting(null);
    },
  });

  if (editing) {
    return (
      <TemplateEditor
        key={editing === "new" ? "new" : editing.id}
        template={editing === "new" ? null : editing}
        saving={save.isPending}
        error={formError}
        onSave={(input) => {
          setFormError(null);
          save.mutate(input);
        }}
        onCancel={() => {
          setEditing(null);
          setFormError(null);
        }}
      />
    );
  }

  const items = list.data ?? [];

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <p className="max-w-xl text-sm text-ink-600">
          {scope.kind === "global"
            ? "Global templates are available when creating a page in any space."
            : `Templates available when creating a page in ${scope.key}.`}
        </p>
        {canManage && (
          <Button variant="primary" onClick={() => setEditing("new")}>
            <Plus size={14} /> New template
          </Button>
        )}
      </div>

      {list.isPending ? (
        <div className="flex justify-center py-16">
          <Spinner size={20} />
        </div>
      ) : list.isError ? (
        <p role="alert" className="text-sm text-danger-600">
          {list.error.message}
        </p>
      ) : items.length === 0 ? (
        <EmptyState
          icon={<FileText size={18} />}
          title="No templates yet"
          description={
            canManage
              ? "Create a template to give authors a ready-made starting point."
              : "Nobody has created a template here yet."
          }
          action={
            canManage ? (
              <Button variant="primary" onClick={() => setEditing("new")}>
                <Plus size={14} /> New template
              </Button>
            ) : undefined
          }
        />
      ) : (
        <ul className="surface divide-y divide-ink-100 overflow-hidden rounded-lg border border-ink-200">
          {items.map((t) => (
            <li
              key={t.id}
              className="flex flex-col gap-2 px-4 py-3 sm:flex-row sm:items-center sm:justify-between"
            >
              <div className="min-w-0">
                <div className="flex items-center gap-2">
                  <button
                    type="button"
                    onClick={() => setPreviewing(t)}
                    className="truncate text-left text-sm font-semibold text-ink-900 hover:text-brand-700 hover:underline"
                  >
                    {t.name}
                  </button>
                  {t.space_id === null && <Badge tone="purple">Global</Badge>}
                </div>
                {t.description && (
                  <p className="mt-0.5 line-clamp-2 text-sm text-ink-600">{t.description}</p>
                )}
                <p className="mt-0.5 text-xs text-ink-500">Updated {formatDate(t.updated_at)}</p>
              </div>
              <div className="flex shrink-0 items-center gap-1">
                <Button size="sm" variant="ghost" onClick={() => setPreviewing(t)}>
                  Preview
                </Button>
                {canManage && (
                  <>
                    <Button size="sm" variant="secondary" onClick={() => setEditing(t)}>
                      <Pencil size={12} /> Edit
                    </Button>
                    <Button size="sm" variant="danger" onClick={() => setDeleting(t)}>
                      <Trash2 size={12} /> Delete
                    </Button>
                  </>
                )}
              </div>
            </li>
          ))}
        </ul>
      )}

      {previewing && (
        <TemplatePreviewModal template={previewing} onClose={() => setPreviewing(null)} />
      )}
      {deleting && (
        <ConfirmDialog
          title="Delete template"
          message={`Delete "${deleting.name}"? Pages already created from it are not affected.`}
          confirmLabel="Delete"
          cancelLabel="Cancel"
          tone="danger"
          busy={remove.isPending}
          onConfirm={() => remove.mutate(deleting)}
          onCancel={() => setDeleting(null)}
        />
      )}
    </div>
  );
}
