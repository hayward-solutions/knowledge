"use client";

import { useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Archive, ArchiveRestore, Trash2 } from "lucide-react";
import {
  archiveSpace,
  deleteSpace,
  unarchiveSpace,
  updateSpace,
  type Space,
  type Visibility,
} from "@/lib/api";
import { Button } from "@/components/ui/Button";
import { ConfirmDialog } from "@/components/ui/ConfirmDialog";
import { Field } from "@/components/ui/Field";
import { Input } from "@/components/ui/Input";
import { Modal } from "@/components/ui/Modal";
import { Select } from "@/components/ui/Select";
import { Textarea } from "@/components/ui/Textarea";
import { useToast } from "@/components/ui/Toast";
import { errorMessage } from "@/components/pages/utils";
import { SettingsShell } from "@/components/spaces/SettingsShell";
import { useSpace } from "@/components/spaces/SpaceContext";
import { buildTree, flattenAll } from "@/components/spaces/tree";

function lastGrapheme(value: string): string {
  const v = value.trim();
  if (!v) return "";
  if (typeof Intl !== "undefined" && "Segmenter" in Intl) {
    const seg = new Intl.Segmenter(undefined, { granularity: "grapheme" });
    let last = "";
    for (const s of seg.segment(v)) last = s.segment;
    return last;
  }
  const chars = Array.from(v);
  return chars[chars.length - 1] ?? "";
}

function GeneralForm({ space }: { space: Space }) {
  const qc = useQueryClient();
  const toast = useToast();
  const { tree, refresh } = useSpace();
  const [name, setName] = useState(space.name);
  const [description, setDescription] = useState(space.description ?? "");
  const [icon, setIcon] = useState(space.icon ?? "");
  const [visibility, setVisibility] = useState<Visibility>(space.visibility);
  const [homeId, setHomeId] = useState(space.home_page_id ?? "");
  const [error, setError] = useState<string | null>(null);

  const pageOptions = useMemo(
    () => flattenAll(buildTree(tree)).filter((n) => n.page.status === "published" || n.page.id === space.home_page_id),
    [tree, space.home_page_id]
  );

  const dirty =
    name !== space.name ||
    description !== (space.description ?? "") ||
    icon !== (space.icon ?? "") ||
    visibility !== space.visibility ||
    homeId !== (space.home_page_id ?? "");

  const save = useMutation({
    mutationFn: () =>
      updateSpace(space.key, {
        name: name.trim(),
        description: description.trim(),
        icon: icon.trim() || null,
        visibility,
        home_page_id: homeId || null,
      }),
    onSuccess: async (updated) => {
      qc.setQueryData(["space", space.key], (prev: Space | undefined) => ({ ...prev, ...updated }));
      toast.success("Settings saved");
      setError(null);
      await Promise.all([qc.invalidateQueries({ queryKey: ["spaces"] }), refresh()]);
    },
    onError: (err) => {
      const msg = errorMessage(err, "Could not save settings");
      setError(msg);
      toast.error(msg);
    },
  });

  return (
    <form
      className="surface space-y-5 p-5"
      onSubmit={(e) => {
        e.preventDefault();
        if (!name.trim()) {
          setError("Name is required");
          return;
        }
        setError(null);
        save.mutate();
      }}
    >
      <div className="grid gap-4 sm:grid-cols-[1fr_96px]">
        <Field label="Name" error={error ?? undefined}>
          <Input value={name} onChange={(e) => setName(e.target.value)} maxLength={200} required />
        </Field>
        <Field label="Icon" hint="One emoji">
          <Input
            value={icon}
            onChange={(e) => setIcon(lastGrapheme(e.target.value))}
            placeholder="📘"
            className="text-center text-lg"
            aria-label="Icon emoji"
          />
        </Field>
      </div>
      <Field label="Description">
        <Textarea value={description} onChange={(e) => setDescription(e.target.value)} rows={3} maxLength={2000} />
      </Field>
      <fieldset>
        <legend className="mb-1 text-xs font-semibold tracking-wide text-ink-700">Visibility</legend>
        <div className="grid gap-2 sm:grid-cols-2">
          {(
            [
              ["internal", "Internal", "Everyone signed in can read this space."],
              ["private", "Private", "Only members can see this space."],
            ] as const
          ).map(([value, label, hint]) => (
            <label
              key={value}
              className={`flex cursor-pointer items-start gap-2 rounded-md border px-3 py-2 text-sm ${
                visibility === value ? "border-brand-500 bg-brand-50" : "border-ink-200 hover:bg-ink-50"
              }`}
            >
              <input
                type="radio"
                name="visibility"
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
      <Field label="Home page" hint="Shown when someone opens the space.">
        <Select value={homeId} onChange={(e) => setHomeId(e.target.value)} className="w-full">
          <option value="">No home page</option>
          {pageOptions.map((n) => (
            <option key={n.page.id} value={n.page.id}>
              {"  ".repeat(n.depth)}
              {n.page.title || "Untitled"}
            </option>
          ))}
        </Select>
      </Field>
      <div className="flex justify-end gap-2">
        <Button
          variant="ghost"
          disabled={!dirty || save.isPending}
          onClick={() => {
            setName(space.name);
            setDescription(space.description ?? "");
            setIcon(space.icon ?? "");
            setVisibility(space.visibility);
            setHomeId(space.home_page_id ?? "");
            setError(null);
          }}
        >
          Reset
        </Button>
        <Button variant="primary" type="submit" disabled={!dirty || save.isPending}>
          {save.isPending ? "Saving..." : "Save changes"}
        </Button>
      </div>
    </form>
  );
}

function DeleteSpaceDialog({ space, onClose }: { space: Space; onClose: () => void }) {
  const router = useRouter();
  const qc = useQueryClient();
  const toast = useToast();
  const [typed, setTyped] = useState("");
  const remove = useMutation({
    mutationFn: () => deleteSpace(space.key),
    onSuccess: async () => {
      toast.success(`Deleted space ${space.name}`);
      qc.removeQueries({ queryKey: ["space", space.key] });
      qc.removeQueries({ queryKey: ["tree", space.key] });
      await qc.invalidateQueries({ queryKey: ["spaces"] });
      onClose();
      router.push("/spaces");
    },
    onError: (err) => toast.error(errorMessage(err, "Could not delete space")),
  });
  const matches = typed === space.key;
  return (
    <Modal
      title="Delete space"
      onClose={onClose}
      width={460}
      footer={
        <>
          <Button variant="ghost" onClick={onClose} disabled={remove.isPending}>
            Cancel
          </Button>
          <Button
            variant="danger"
            disabled={!matches || remove.isPending}
            onClick={() => remove.mutate()}
          >
            {remove.isPending ? "Deleting..." : "Delete space"}
          </Button>
        </>
      }
    >
      <form
        className="space-y-3"
        onSubmit={(e) => {
          e.preventDefault();
          if (matches) remove.mutate();
        }}
      >
        <p className="text-sm text-ink-700">
          <strong>{space.name}</strong> and all of its pages will no longer be accessible.
        </p>
        <Field label={`Type ${space.key} to confirm`}>
          <Input
            data-autofocus
            value={typed}
            onChange={(e) => setTyped(e.target.value)}
            autoComplete="off"
            spellCheck={false}
          />
        </Field>
      </form>
    </Modal>
  );
}

export default function GeneralSettingsPage() {
  const qc = useQueryClient();
  const toast = useToast();
  const { space, refresh } = useSpace();
  const [confirm, setConfirm] = useState<"archive" | "delete" | null>(null);
  const archived = !!space.archived_at;

  const toggleArchive = useMutation({
    mutationFn: () => (archived ? unarchiveSpace(space.key) : archiveSpace(space.key)),
    onSuccess: async () => {
      toast.success(archived ? "Space unarchived" : "Space archived");
      setConfirm(null);
      await Promise.all([qc.invalidateQueries({ queryKey: ["spaces"] }), refresh()]);
    },
    onError: (err) => {
      setConfirm(null);
      toast.error(errorMessage(err, "Could not update space"));
    },
  });

  return (
    <SettingsShell active="general">
      <GeneralForm key={`${space.id}-${space.updated_at}`} space={space} />

      <section className="surface mt-6 p-5">
        <h2 className="text-sm font-semibold text-ink-900">{archived ? "Unarchive space" : "Archive space"}</h2>
        <p className="mt-1 text-sm text-ink-600">
          {archived
            ? "Unarchiving makes the space editable again."
            : "Archived spaces stay readable but nobody can edit pages, upload attachments or change members until it is unarchived."}
        </p>
        <div className="mt-3">
          <Button onClick={() => (archived ? toggleArchive.mutate() : setConfirm("archive"))} disabled={toggleArchive.isPending}>
            {archived ? <ArchiveRestore size={14} /> : <Archive size={14} />}
            {archived ? "Unarchive space" : "Archive space"}
          </Button>
        </div>
      </section>

      <section className="mt-6 rounded-lg border border-danger-100 bg-ink-0 p-5">
        <h2 className="text-sm font-semibold text-danger-600">Delete space</h2>
        <p className="mt-1 text-sm text-ink-600">
          Deleting a space removes it from the directory for everyone. You will be asked to type the space key to
          confirm.
        </p>
        <div className="mt-3">
          <Button variant="danger" onClick={() => setConfirm("delete")}>
            <Trash2 size={14} />
            Delete space
          </Button>
        </div>
      </section>

      {confirm === "archive" && (
        <ConfirmDialog
          title="Archive space"
          confirmLabel="Archive"
          busy={toggleArchive.isPending}
          onConfirm={() => toggleArchive.mutate()}
          onCancel={() => setConfirm(null)}
          message={<>{space.name} will become read-only. You can unarchive it at any time.</>}
        />
      )}
      {confirm === "delete" && <DeleteSpaceDialog space={space} onClose={() => setConfirm(null)} />}
    </SettingsShell>
  );
}
