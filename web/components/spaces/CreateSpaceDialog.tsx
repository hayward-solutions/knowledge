"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { ApiError, createSpace, type Visibility } from "@/lib/api";
import { cn } from "@/lib/cn";
import { Button } from "@/components/ui/Button";
import { Field } from "@/components/ui/Field";
import { Input } from "@/components/ui/Input";
import { Modal } from "@/components/ui/Modal";
import { Textarea } from "@/components/ui/Textarea";
import { useToast } from "@/components/ui/Toast";

const KEY_PATTERN = /^[A-Z][A-Z0-9]{1,9}$/;

export function suggestKey(name: string): string {
  const words = name.toUpperCase().match(/[A-Z0-9]+/g) ?? [];
  if (words.length === 0) return "";
  let key = words.length === 1 ? words[0].slice(0, 5) : words.map((w) => w[0]).join("").slice(0, 5);
  key = key.replace(/^[0-9]+/, "");
  if (key.length < 2) {
    const first = words.find((w) => /[A-Z]/.test(w));
    key = first ? first.replace(/^[0-9]+/, "").slice(0, 3) : "";
  }
  return key;
}

function cleanKey(raw: string): string {
  return raw.toUpperCase().replace(/[^A-Z0-9]/g, "").slice(0, 10);
}

export function CreateSpaceDialog({ onClose }: { onClose: () => void }) {
  const router = useRouter();
  const qc = useQueryClient();
  const toast = useToast();
  const [name, setName] = useState("");
  const [key, setKey] = useState("");
  const [keyTouched, setKeyTouched] = useState(false);
  const [description, setDescription] = useState("");
  const [visibility, setVisibility] = useState<Visibility>("internal");
  const [nameError, setNameError] = useState<string | null>(null);
  const [keyError, setKeyError] = useState<string | null>(null);
  const [formError, setFormError] = useState<string | null>(null);

  const create = useMutation({
    mutationFn: () =>
      createSpace({ key, name: name.trim(), description: description.trim(), visibility }),
    onSuccess: async (space) => {
      await qc.invalidateQueries({ queryKey: ["spaces"] });
      toast.success(`Created space ${space.name}`);
      onClose();
      router.push(`/spaces/${space.key}`);
    },
    onError: (err) => {
      if (err instanceof ApiError && err.status === 409) {
        setKeyError("That key is already taken. Choose another.");
        return;
      }
      if (err instanceof ApiError && err.status === 400) {
        const msg = err.message || "Check the details and try again.";
        if (/key/i.test(msg)) setKeyError(msg);
        else if (/name/i.test(msg)) setNameError(msg);
        else setFormError(msg);
        return;
      }
      setFormError(err instanceof Error ? err.message : "Could not create space");
    },
  });

  function onNameChange(value: string) {
    setName(value);
    setNameError(null);
    if (!keyTouched) {
      setKey(suggestKey(value));
      setKeyError(null);
    }
  }

  function submit(e: React.FormEvent) {
    e.preventDefault();
    setFormError(null);
    let ok = true;
    if (!name.trim()) {
      setNameError("Give the space a name");
      ok = false;
    }
    if (!KEY_PATTERN.test(key)) {
      setKeyError("Use 2 to 10 letters or digits, starting with a letter");
      ok = false;
    }
    if (ok) create.mutate();
  }

  return (
    <Modal
      title="Create space"
      onClose={onClose}
      width={520}
      footer={
        <>
          <Button variant="ghost" onClick={onClose} disabled={create.isPending}>
            Cancel
          </Button>
          <Button variant="primary" type="submit" form="create-space-form" disabled={create.isPending}>
            {create.isPending ? "Creating..." : "Create space"}
          </Button>
        </>
      }
    >
      <form id="create-space-form" onSubmit={submit} className="space-y-4" noValidate>
        {formError && (
          <div role="alert" className="rounded-md border border-danger-100 bg-danger-50 px-3 py-2 text-sm text-danger-700">
            {formError}
          </div>
        )}
        <Field label="Name" error={nameError ?? undefined}>
          <Input
            data-autofocus
            value={name}
            onChange={(e) => onNameChange(e.target.value)}
            placeholder="Engineering"
            maxLength={200}
          />
        </Field>
        <Field
          label="Key"
          hint="Short, unique and permanent. Used in links, for example ENG."
          error={keyError ?? undefined}
        >
          <Input
            value={key}
            onChange={(e) => {
              setKeyTouched(true);
              setKey(cleanKey(e.target.value));
              setKeyError(null);
            }}
            placeholder="ENG"
            className="font-mono uppercase"
            autoComplete="off"
            spellCheck={false}
          />
        </Field>
        <Field label="Description">
          <Textarea
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            rows={3}
            maxLength={2000}
            placeholder="What is this space for?"
          />
        </Field>
        <fieldset>
          <legend className="mb-1 text-xs font-semibold tracking-wide text-ink-700">Visibility</legend>
          <div className="grid gap-2 sm:grid-cols-2">
            {(
              [
                ["internal", "Internal", "Everyone signed in can read it."],
                ["private", "Private", "Only members can see it."],
              ] as const
            ).map(([value, label, hint]) => (
              <label
                key={value}
                className={cn(
                  "flex cursor-pointer items-start gap-2 rounded-md border px-3 py-2 text-sm",
                  visibility === value ? "border-brand-500 bg-brand-50" : "border-ink-200 hover:bg-ink-50"
                )}
              >
                <input
                  type="radio"
                  name="space-visibility"
                  checked={visibility === value}
                  onChange={() => setVisibility(value)}
                  className="mt-1"
                />
                <span>
                  <span className="block font-medium text-ink-900">{label}</span>
                  <span className="block text-xs text-ink-600">{hint}</span>
                </span>
              </label>
            ))}
          </div>
        </fieldset>
      </form>
    </Modal>
  );
}
