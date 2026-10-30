"use client";

import { useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { FileText } from "lucide-react";
import { createPage, listTemplates, type Template } from "@/lib/api";
import { cn } from "@/lib/cn";
import { Button } from "@/components/ui/Button";
import { Field } from "@/components/ui/Field";
import { Input } from "@/components/ui/Input";
import { Modal } from "@/components/ui/Modal";
import { useToast } from "@/components/ui/Toast";
import { useSpace } from "@/components/spaces/SpaceContext";
import { ParentSelect } from "./ParentSelect";
import { errorMessage, pageEditHref } from "./utils";

export function NewPageDialog({
  defaultParentId,
  onClose,
}: {
  defaultParentId?: string | null;
  onClose: () => void;
}) {
  const router = useRouter();
  const qc = useQueryClient();
  const toast = useToast();
  const { space, tree } = useSpace();
  const [title, setTitle] = useState("");
  const [parentId, setParentId] = useState(
    () =>
      (defaultParentId && tree.some((p) => p.id === defaultParentId) ? defaultParentId : null) ??
      space.home_page_id ??
      tree.find((p) => !p.parent_id)?.id ??
      ""
  );
  const [templateId, setTemplateId] = useState<string>("");
  const [error, setError] = useState<string | null>(null);

  const templatesQuery = useQuery<Template[], Error>({
    queryKey: ["templates"],
    queryFn: listTemplates,
    staleTime: 30_000,
  });
  const templates = useMemo(
    () => (templatesQuery.data ?? []).filter((t) => t.space_id === null || t.space_id === space.id),
    [templatesQuery.data, space.id]
  );

  const create = useMutation({
    mutationFn: () =>
      createPage(space.key, {
        title: title.trim(),
        parent_id: parentId || undefined,
        template_id: templateId || undefined,
      }),
    onSuccess: async (page) => {
      await qc.invalidateQueries({ queryKey: ["tree", space.key] });
      void qc.invalidateQueries({ queryKey: ["dashboard"] });
      onClose();
      router.push(pageEditHref(space.key, page.id));
    },
    onError: (err) => {
      const msg = errorMessage(err, "Could not create page");
      setError(msg);
      toast.error(msg);
    },
  });

  function submit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    create.mutate();
  }

  return (
    <Modal
      title="Create page"
      onClose={onClose}
      width={520}
      footer={
        <>
          <Button variant="ghost" onClick={onClose} disabled={create.isPending}>
            Cancel
          </Button>
          <Button variant="primary" type="submit" form="new-page-form" disabled={create.isPending}>
            {create.isPending ? "Creating..." : "Create"}
          </Button>
        </>
      }
    >
      <form id="new-page-form" onSubmit={submit} className="space-y-4">
        <Field label="Title" error={error ?? undefined}>
          <Input
            data-autofocus
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            placeholder="Untitled"
            maxLength={250}
          />
        </Field>
        <Field label="Parent page">
          <ParentSelect pages={tree} value={parentId} onChange={setParentId} />
        </Field>
        {templates.length > 0 && (
          <fieldset>
            <legend className="mb-1 block text-xs font-semibold tracking-wide text-ink-700">
              Template
            </legend>
            <div className="grid max-h-56 grid-cols-1 gap-1.5 overflow-y-auto overflow-x-hidden p-0.5">
              {[{ id: "", name: "Blank page", description: "Start with an empty page" }, ...templates].map(
                (t) => (
                  <label
                    key={t.id || "blank"}
                    className={cn(
                      "flex min-w-0 cursor-pointer items-start gap-2 rounded-md border px-3 py-2 text-sm transition",
                      templateId === t.id
                        ? "border-brand-500 bg-brand-50"
                        : "border-ink-200 hover:bg-ink-50"
                    )}
                  >
                    <input
                      type="radio"
                      name="template"
                      checked={templateId === t.id}
                      onChange={() => setTemplateId(t.id)}
                      className="mt-1"
                    />
                    <FileText size={16} className="mt-0.5 shrink-0 text-ink-500" />
                    <span className="min-w-0">
                      <span className="block font-medium text-ink-900">{t.name}</span>
                      {t.description && (
                        <span className="block truncate text-xs text-ink-600">{t.description}</span>
                      )}
                    </span>
                  </label>
                )
              )}
            </div>
          </fieldset>
        )}
      </form>
    </Modal>
  );
}
