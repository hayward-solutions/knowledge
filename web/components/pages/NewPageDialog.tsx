"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { createPage } from "@/lib/api";
import { Button } from "@/components/ui/Button";
import { Field } from "@/components/ui/Field";
import { Input } from "@/components/ui/Input";
import { Modal } from "@/components/ui/Modal";
import { useToast } from "@/components/ui/Toast";
import { useSpace } from "@/components/spaces/SpaceContext";
import { TemplatePicker } from "@/components/templates/TemplatePicker";
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
        <Field label="Template">
          <TemplatePicker
            spaceKey={space.key}
            value={templateId || null}
            onChange={(id) => setTemplateId(id ?? "")}
            disabled={create.isPending}
          />
        </Field>
      </form>
    </Modal>
  );
}
