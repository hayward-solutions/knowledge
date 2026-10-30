"use client";

import { useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { useMutation, useQuery } from "@tanstack/react-query";
import { createPage, getSpaceTree, listSpaces, type Template } from "@/lib/api";
import { useMe } from "@/lib/auth";
import { Button } from "@/components/ui/Button";
import { Field } from "@/components/ui/Field";
import { Input } from "@/components/ui/Input";
import { Modal } from "@/components/ui/Modal";
import { Select } from "@/components/ui/Select";
import { indentLabel, flattenPages } from "./pageOptions";

export function UseTemplateDialog({
  template,
  onClose,
  defaultSpaceKey,
}: {
  template: Template;
  onClose: () => void;
  defaultSpaceKey?: string;
}) {
  const router = useRouter();
  const { data: me } = useMe();
  const spaces = useQuery({ queryKey: ["spaces"], queryFn: listSpaces, staleTime: 60_000 });

  const writable = useMemo(
    () =>
      (spaces.data ?? []).filter(
        (s) =>
          !s.archived_at &&
          (me?.is_system_admin || s.role === "admin" || s.role === "editor")
      ),
    [spaces.data, me?.is_system_admin]
  );

  const [spaceKey, setSpaceKey] = useState(defaultSpaceKey ?? template.space_key ?? "");
  const [parentId, setParentId] = useState("");
  const [title, setTitle] = useState(template.name);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    if (spaceKey && writable.some((s) => s.key === spaceKey)) return;
    if (writable.length > 0) setSpaceKey(writable[0].key);
  }, [writable, spaceKey]);

  const tree = useQuery({
    queryKey: ["templates", "tree", spaceKey],
    queryFn: () => getSpaceTree(spaceKey),
    enabled: Boolean(spaceKey),
  });
  const options = useMemo(() => flattenPages(tree.data ?? []), [tree.data]);

  const create = useMutation({
    mutationFn: () =>
      createPage(spaceKey, {
        title: title.trim(),
        parent_id: parentId || null,
        template_id: template.id,
      }),
    onSuccess: (page) => {
      router.push(`/spaces/${spaceKey}/pages/${page.id}/edit`);
      onClose();
    },
    onError: (e: Error) => setErr(e.message),
  });

  const ready = Boolean(spaceKey) && title.trim().length > 0 && !create.isPending;

  return (
    <Modal
      title={`Use template: ${template.name}`}
      onClose={onClose}
      width={520}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" disabled={!ready} onClick={() => create.mutate()}>
            {create.isPending ? "Creating…" : "Create page"}
          </Button>
        </>
      }
    >
      <form
        className="space-y-3"
        onSubmit={(e) => {
          e.preventDefault();
          if (ready) create.mutate();
        }}
      >
        {spaces.isSuccess && writable.length === 0 ? (
          <p className="text-sm text-ink-600">
            You do not have edit access to any active space, so there is nowhere to create a page.
          </p>
        ) : (
          <>
            <Field label="Space">
              <Select
                value={spaceKey}
                onChange={(e) => {
                  setSpaceKey(e.target.value);
                  setParentId("");
                }}
              >
                {writable.map((s) => (
                  <option key={s.id} value={s.key}>
                    {s.name} ({s.key})
                  </option>
                ))}
              </Select>
            </Field>
            <Field label="Parent page" hint="Leave as space root to create a top-level page.">
              <Select
                value={parentId}
                onChange={(e) => setParentId(e.target.value)}
                disabled={tree.isPending && Boolean(spaceKey)}
              >
                <option value="">Space root</option>
                {options.map((o) => (
                  <option key={o.id} value={o.id}>
                    {indentLabel(o)}
                  </option>
                ))}
              </Select>
            </Field>
            <Field label="Title">
              <Input
                data-autofocus
                value={title}
                onChange={(e) => setTitle(e.target.value)}
                maxLength={200}
              />
            </Field>
          </>
        )}
        {err && (
          <div
            role="alert"
            className="rounded-md border border-danger-200 bg-danger-50 px-3 py-2 text-sm text-danger-700"
          >
            {err}
          </div>
        )}
      </form>
    </Modal>
  );
}
