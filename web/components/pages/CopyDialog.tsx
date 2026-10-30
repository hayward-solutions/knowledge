"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { copyPage } from "@/lib/api";
import { Button } from "@/components/ui/Button";
import { Field } from "@/components/ui/Field";
import { Input } from "@/components/ui/Input";
import { Modal } from "@/components/ui/Modal";
import { useToast } from "@/components/ui/Toast";
import { useSpace } from "@/components/spaces/SpaceContext";
import { ParentSelect } from "./ParentSelect";
import { errorMessage, pageEditHref } from "./utils";

export function CopyDialog({ pageId, onClose }: { pageId: string; onClose: () => void }) {
  const router = useRouter();
  const qc = useQueryClient();
  const toast = useToast();
  const { space, tree } = useSpace();
  const source = tree.find((p) => p.id === pageId);
  const [title, setTitle] = useState(`Copy of ${source?.title ?? "page"}`);
  const [parentId, setParentId] = useState(source?.parent_id ?? space.home_page_id ?? "");
  const [error, setError] = useState<string | null>(null);

  const copy = useMutation({
    mutationFn: () =>
      copyPage(pageId, { title: title.trim() || undefined, parent_id: parentId || undefined }),
    onSuccess: async (page) => {
      await qc.invalidateQueries({ queryKey: ["tree", space.key] });
      void qc.invalidateQueries({ queryKey: ["dashboard"] });
      toast.success("Page copied. The copy is a draft until you publish it.");
      onClose();
      router.push(pageEditHref(space.key, page.id));
    },
    onError: (err) => {
      const msg = errorMessage(err, "Could not copy page");
      setError(msg);
      toast.error(msg);
    },
  });

  return (
    <Modal
      title="Copy page"
      onClose={onClose}
      width={480}
      footer={
        <>
          <Button variant="ghost" onClick={onClose} disabled={copy.isPending}>
            Cancel
          </Button>
          <Button variant="primary" type="submit" form="copy-page-form" disabled={copy.isPending}>
            {copy.isPending ? "Copying..." : "Copy"}
          </Button>
        </>
      }
    >
      <form
        id="copy-page-form"
        className="space-y-4"
        onSubmit={(e) => {
          e.preventDefault();
          setError(null);
          copy.mutate();
        }}
      >
        <Field label="Title" error={error ?? undefined}>
          <Input data-autofocus value={title} onChange={(e) => setTitle(e.target.value)} maxLength={250} />
        </Field>
        <Field label="Parent page">
          <ParentSelect pages={tree} value={parentId} onChange={setParentId} />
        </Field>
        <p className="text-xs text-ink-600">
          Only this page is copied, not its child pages. Attachments are not copied.
        </p>
      </form>
    </Modal>
  );
}
