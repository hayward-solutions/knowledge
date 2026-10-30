"use client";

import { useRef, useState } from "react";
import { Editor, type EditorHandle } from "@/components/editor";
import type { Template, TemplateInput } from "@/lib/api";
import { Button } from "@/components/ui/Button";
import { Field } from "@/components/ui/Field";
import { Input } from "@/components/ui/Input";

export interface TemplateEditorProps {
  template?: Template | null;
  saving?: boolean;
  error?: string | null;
  onSave: (input: TemplateInput) => void;
  onCancel: () => void;
}

export function TemplateEditor({
  template,
  saving = false,
  error,
  onSave,
  onCancel,
}: TemplateEditorProps) {
  const handle = useRef<EditorHandle>(null);
  const [name, setName] = useState(template?.name ?? "");
  const [description, setDescription] = useState(template?.description ?? "");

  const canSave = name.trim().length > 0 && !saving;

  const submit = () => {
    if (!canSave) return;
    const value = handle.current?.getValue();
    onSave({
      name: name.trim(),
      description: description.trim(),
      content_json: value?.json ?? template?.content_json ?? null,
      content_html: value?.html ?? template?.content_html ?? "",
    });
  };

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 className="text-base font-semibold text-ink-900">
          {template ? "Edit template" : "New template"}
        </h2>
        <div className="flex items-center gap-2">
          <Button variant="ghost" onClick={onCancel} disabled={saving}>
            Cancel
          </Button>
          <Button variant="primary" onClick={submit} disabled={!canSave}>
            {saving ? "Saving…" : "Save template"}
          </Button>
        </div>
      </div>

      {error && (
        <div
          role="alert"
          className="rounded-md border border-danger-200 bg-danger-50 px-3 py-2 text-sm text-danger-700"
        >
          {error}
        </div>
      )}

      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="Name">
          <Input
            data-autofocus
            value={name}
            onChange={(e) => setName(e.target.value)}
            maxLength={120}
            placeholder="Meeting notes"
          />
        </Field>
        <Field label="Description">
          <Input
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            maxLength={2000}
            placeholder="When to use this template"
          />
        </Field>
      </div>

      <div className="rounded-lg border border-ink-200 bg-ink-0">
        <Editor
          ref={handle}
          variant="full"
          value={{ json: template?.content_json ?? null, html: template?.content_html ?? "" }}
          placeholder="Write the template content. Type / for commands."
          ariaLabel="Template content"
          className="px-4 py-3 sm:px-6"
        />
      </div>
    </div>
  );
}
