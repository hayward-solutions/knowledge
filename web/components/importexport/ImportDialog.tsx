"use client";

import { useMemo, useRef, useState } from "react";
import Link from "next/link";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CheckCircle2, FileText, FolderUp, Upload, X } from "lucide-react";
import { getSpaceTree, importMarkdown, type ImportResult } from "@/lib/api";
import { slugify } from "@/lib/content";
import { flattenPages, indentLabel } from "@/components/templates/pageOptions";
import { Button } from "@/components/ui/Button";
import { Field } from "@/components/ui/Field";
import { Modal } from "@/components/ui/Modal";
import { Select } from "@/components/ui/Select";

const MARKDOWN = /\.(md|markdown)$/i;

function labelOf(f: File): string {
  return f.webkitRelativePath || f.name;
}

export function ImportDialog({
  spaceKey,
  onClose,
  onImported,
}: {
  spaceKey: string;
  onClose: () => void;
  onImported?: (result: ImportResult) => void;
}) {
  const qc = useQueryClient();
  const filesRef = useRef<HTMLInputElement>(null);
  const folderRef = useRef<HTMLInputElement | null>(null);
  const [files, setFiles] = useState<File[]>([]);
  const [skipped, setSkipped] = useState(0);
  const [parentId, setParentId] = useState("");
  const [result, setResult] = useState<ImportResult | null>(null);
  const [err, setErr] = useState<string | null>(null);

  const tree = useQuery({
    queryKey: ["import", "tree", spaceKey],
    queryFn: () => getSpaceTree(spaceKey),
  });
  const options = useMemo(() => flattenPages(tree.data ?? []), [tree.data]);

  const accept = (list: FileList | null, replace: boolean) => {
    const all = Array.from(list ?? []);
    const md = all.filter((f) => MARKDOWN.test(f.name));
    setSkipped(all.length - md.length);
    setFiles((prev) => {
      const base = replace ? [] : prev;
      const seen = new Set(base.map(labelOf));
      return [...base, ...md.filter((f) => !seen.has(labelOf(f)))];
    });
    setErr(null);
  };

  const run = useMutation({
    mutationFn: () => importMarkdown(spaceKey, files, parentId || null),
    onSuccess: (res) => {
      setResult(res);
      onImported?.(res);
      qc.invalidateQueries();
    },
    onError: (e: Error) => setErr(e.message),
  });

  const created = result?.pages ?? [];
  const importLabel =
    files.length === 0 ? "Import" : `Import ${files.length} ${files.length === 1 ? "file" : "files"}`;

  return (
    <Modal
      title="Import Markdown"
      onClose={onClose}
      width={560}
      footer={
        result ? (
          <Button variant="primary" onClick={onClose}>
            Done
          </Button>
        ) : (
          <>
            <Button variant="ghost" onClick={onClose} disabled={run.isPending}>
              Cancel
            </Button>
            <Button
              variant="primary"
              disabled={files.length === 0 || run.isPending}
              onClick={() => run.mutate()}
            >
              <Upload size={14} />
              {run.isPending ? "Importing…" : importLabel}
            </Button>
          </>
        )
      }
    >
      {result ? (
        <div className="space-y-3">
          <div className="flex items-center gap-2 text-sm font-medium text-success-700">
            <CheckCircle2 size={16} />
            Created {created.length} {created.length === 1 ? "page" : "pages"}
          </div>
          {created.length > 0 && (
            <ul className="max-h-72 divide-y divide-ink-100 overflow-y-auto rounded-md border border-ink-200">
              {created.map((p) => (
                <li key={p.id}>
                  <Link
                    href={`/spaces/${spaceKey}/pages/${p.id}/${p.slug || slugify(p.title)}`}
                    onClick={onClose}
                    className="flex items-center gap-2 px-3 py-2 text-sm text-brand-700 hover:bg-ink-50 hover:underline"
                  >
                    <FileText size={14} className="shrink-0 text-ink-500" />
                    <span className="truncate">{p.title || "Untitled"}</span>
                  </Link>
                </li>
              ))}
            </ul>
          )}
        </div>
      ) : (
        <div className="space-y-4">
          <input
            ref={filesRef}
            type="file"
            multiple
            accept=".md,.markdown,text/markdown"
            className="sr-only"
            tabIndex={-1}
            onChange={(e) => {
              accept(e.target.files, false);
              e.target.value = "";
            }}
          />
          <input
            ref={(el) => {
              folderRef.current = el;
              if (el) {
                el.setAttribute("webkitdirectory", "");
                el.setAttribute("directory", "");
              }
            }}
            type="file"
            multiple
            className="sr-only"
            tabIndex={-1}
            onChange={(e) => {
              accept(e.target.files, false);
              e.target.value = "";
            }}
          />

          <div className="flex flex-wrap gap-2">
            <Button onClick={() => filesRef.current?.click()}>
              <FileText size={14} /> Choose files
            </Button>
            <Button onClick={() => folderRef.current?.click()}>
              <FolderUp size={14} /> Choose folder
            </Button>
          </div>
          <p className="text-xs text-ink-500">
            Each .md file becomes a page. Folders become nested child pages.
          </p>

          {files.length > 0 && (
            <div className="rounded-md border border-ink-200">
              <div className="flex items-center justify-between border-b border-ink-200 bg-ink-50 px-3 py-1.5 text-xs font-medium text-ink-700">
                <span>
                  {files.length} {files.length === 1 ? "file" : "files"} selected
                </span>
                <button
                  type="button"
                  onClick={() => {
                    setFiles([]);
                    setSkipped(0);
                  }}
                  className="text-ink-600 hover:text-ink-900 hover:underline"
                >
                  Clear
                </button>
              </div>
              <ul className="max-h-44 divide-y divide-ink-100 overflow-y-auto">
                {files.map((f) => (
                  <li
                    key={labelOf(f)}
                    className="flex items-center justify-between gap-2 px-3 py-1.5 text-sm text-ink-800"
                  >
                    <span className="truncate">{labelOf(f)}</span>
                    <button
                      type="button"
                      aria-label={`Remove ${labelOf(f)}`}
                      onClick={() => setFiles((prev) => prev.filter((x) => x !== f))}
                      className="shrink-0 rounded-xs p-0.5 text-ink-500 hover:bg-ink-100 hover:text-ink-900"
                    >
                      <X size={14} />
                    </button>
                  </li>
                ))}
              </ul>
            </div>
          )}
          {skipped > 0 && (
            <p className="text-xs text-warning-700">
              Skipped {skipped} {skipped === 1 ? "file" : "files"} that are not Markdown.
            </p>
          )}

          <Field label="Parent page" hint="Imported pages are created under this page.">
            <Select value={parentId} onChange={(e) => setParentId(e.target.value)}>
              <option value="">Space root</option>
              {options.map((o) => (
                <option key={o.id} value={o.id}>
                  {indentLabel(o)}
                </option>
              ))}
            </Select>
          </Field>

          {err && (
            <div
              role="alert"
              className="rounded-md border border-danger-200 bg-danger-50 px-3 py-2 text-sm text-danger-700"
            >
              {err}
            </div>
          )}
        </div>
      )}
    </Modal>
  );
}
