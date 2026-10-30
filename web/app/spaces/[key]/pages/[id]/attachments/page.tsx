"use client";

import { useRef, useState } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, Code2, Download, FileIcon, Hash, Paperclip, Trash2, Upload } from "lucide-react";
import {
  ApiError,
  attachmentContentUrl,
  attachmentDownloadUrl,
  deleteAttachment,
  getPage,
  listAttachments,
  uploadAttachment,
  type Attachment,
  type PageDetail,
} from "@/lib/api";
import { cn } from "@/lib/cn";
import { Avatar } from "@/components/ui/Avatar";
import { Button } from "@/components/ui/Button";
import { ConfirmDialog } from "@/components/ui/ConfirmDialog";
import { EmptyState } from "@/components/ui/EmptyState";
import { Spinner } from "@/components/ui/Spinner";
import { useToast } from "@/components/ui/Toast";
import { PageBreadcrumbs } from "@/components/pages/Breadcrumbs";
import { RelativeTime } from "@/components/pages/RelativeTime";
import { ListSkeleton } from "@/components/pages/Skeletons";
import { absoluteUrl, errorMessage, formatBytes, pageHref } from "@/components/pages/utils";
import { useSpace } from "@/components/spaces/SpaceContext";

interface PendingUpload {
  key: number;
  name: string;
  error: string | null;
}

function isImage(a: Attachment): boolean {
  return a.content_type.startsWith("image/");
}

export default function AttachmentsPage() {
  const params = useParams<{ id: string }>();
  const id = String(params.id);
  const qc = useQueryClient();
  const toast = useToast();
  const { space, canEdit } = useSpace();
  const inputRef = useRef<HTMLInputElement>(null);
  const counter = useRef(0);
  const [pending, setPending] = useState<PendingUpload[]>([]);
  const [dragging, setDragging] = useState(false);
  const [toDelete, setToDelete] = useState<Attachment | null>(null);

  const pageQuery = useQuery<PageDetail, Error>({
    queryKey: ["page", id],
    queryFn: () => getPage(id),
    retry: false,
  });
  const listQuery = useQuery<Attachment[], Error>({
    queryKey: ["attachments", id],
    queryFn: () => listAttachments(id),
    retry: false,
  });

  const page = pageQuery.data;
  const writable = canEdit && page?.status !== "trashed";

  const remove = useMutation({
    mutationFn: (a: Attachment) => deleteAttachment(a.id),
    onSuccess: async () => {
      toast.success("Attachment deleted");
      setToDelete(null);
      await qc.invalidateQueries({ queryKey: ["attachments", id] });
    },
    onError: (err) => {
      setToDelete(null);
      toast.error(errorMessage(err, "Could not delete attachment"));
    },
  });

  async function uploadFiles(files: File[]) {
    if (files.length === 0 || !writable) return;
    const items = files.map((f) => ({ key: ++counter.current, file: f }));
    setPending((cur) => [...cur, ...items.map((i) => ({ key: i.key, name: i.file.name, error: null }))]);
    let uploaded = 0;
    for (const item of items) {
      try {
        await uploadAttachment(id, item.file);
        uploaded++;
        setPending((cur) => cur.filter((p) => p.key !== item.key));
      } catch (err) {
        const msg =
          err instanceof ApiError && err.status === 413
            ? "File is too large"
            : errorMessage(err, "Upload failed");
        setPending((cur) => cur.map((p) => (p.key === item.key ? { ...p, error: msg } : p)));
      }
    }
    if (uploaded > 0) {
      toast.success(uploaded === 1 ? "File uploaded" : `${uploaded} files uploaded`);
      await qc.invalidateQueries({ queryKey: ["attachments", id] });
    }
  }

  function copy(text: string, label: string) {
    navigator.clipboard
      ?.writeText(text)
      .then(() => toast.success(`${label} copied`))
      .catch(() => toast.error("Could not copy to the clipboard"));
  }

  const backHref = page ? pageHref(space.key, page) : `/spaces/${space.key}/pages/${id}`;

  if (listQuery.isError) {
    const status = listQuery.error instanceof ApiError ? listQuery.error.status : 0;
    return (
      <div className="mx-auto w-full max-w-xl px-4 py-16">
        <EmptyState
          title={status === 404 ? "Page not found" : "Could not load attachments"}
          description={status === 404 ? "This page does not exist, or you do not have access to it." : listQuery.error.message}
          action={
            status === 404 ? (
              <Link href={`/spaces/${space.key}`} className="btn btn-primary">
                Go to space home
              </Link>
            ) : (
              <Button variant="primary" onClick={() => void listQuery.refetch()}>
                Try again
              </Button>
            )
          }
        />
      </div>
    );
  }

  const items = listQuery.data ?? [];

  return (
    <div className="flex-1 bg-ink-0 px-4 py-6 sm:px-8">
      <div className="mx-auto w-full max-w-5xl">
        <PageBreadcrumbs space={space} ancestors={page?.breadcrumbs ?? []} title={page?.title} />
        <div className="mt-4 flex flex-wrap items-center justify-between gap-3">
          <div className="flex min-w-0 items-center gap-2">
            <Paperclip size={20} className="shrink-0 text-ink-500" />
            <h1 className="truncate text-[22px] font-semibold tracking-tight text-ink-900">Attachments</h1>
          </div>
          <div className="flex items-center gap-2">
            {writable && (
              <Button variant="primary" onClick={() => inputRef.current?.click()}>
                <Upload size={14} />
                Upload files
              </Button>
            )}
            <Link href={backHref} className="btn btn-secondary">
              <ArrowLeft size={14} />
              Back to page
            </Link>
          </div>
        </div>

        {writable && (
          <>
            <input
              ref={inputRef}
              type="file"
              multiple
              className="sr-only"
              tabIndex={-1}
              onChange={(e) => {
                const files = Array.from(e.target.files ?? []);
                e.target.value = "";
                void uploadFiles(files);
              }}
            />
            <div
              onDragOver={(e) => {
                e.preventDefault();
                setDragging(true);
              }}
              onDragLeave={() => setDragging(false)}
              onDrop={(e) => {
                e.preventDefault();
                setDragging(false);
                void uploadFiles(Array.from(e.dataTransfer.files));
              }}
              className={cn(
                "mt-5 flex flex-col items-center justify-center gap-1 rounded-lg border-2 border-dashed px-4 py-8 text-center text-sm transition",
                dragging ? "border-brand-500 bg-brand-50 text-brand-700" : "border-ink-200 text-ink-600"
              )}
            >
              <Upload size={20} className="text-ink-400" />
              <span>
                Drop files here or{" "}
                <button type="button" className="link font-medium" onClick={() => inputRef.current?.click()}>
                  browse
                </button>
              </span>
            </div>
          </>
        )}

        {pending.length > 0 && (
          <ul className="mt-3 space-y-1.5" aria-live="polite">
            {pending.map((p) => (
              <li
                key={p.key}
                className={cn(
                  "flex items-center gap-2 rounded-md border px-3 py-2 text-sm",
                  p.error ? "border-danger-100 bg-danger-50 text-danger-700" : "border-ink-200 text-ink-700"
                )}
              >
                {p.error ? null : <Spinner size={14} label={`Uploading ${p.name}`} />}
                <span className="min-w-0 flex-1 truncate">{p.name}</span>
                {p.error ? (
                  <>
                    <span>{p.error}</span>
                    <button
                      type="button"
                      className="font-medium underline"
                      onClick={() => setPending((cur) => cur.filter((x) => x.key !== p.key))}
                    >
                      Dismiss
                    </button>
                  </>
                ) : (
                  <span className="text-xs text-ink-500">Uploading...</span>
                )}
              </li>
            ))}
          </ul>
        )}

        <div className="mt-6">
          {listQuery.isPending ? (
            <ListSkeleton rows={4} />
          ) : items.length === 0 ? (
            <EmptyState
              icon={<Paperclip size={18} />}
              title="No attachments yet"
              description={
                writable
                  ? "Upload files to attach them to this page. Images can be embedded in the page from the editor."
                  : "Nothing has been attached to this page."
              }
            />
          ) : (
            <ul className="surface divide-y divide-ink-100 overflow-hidden">
              {items.map((a) => (
                <li key={a.id} className="flex flex-wrap items-center gap-3 px-4 py-3">
                  <span className="flex h-12 w-12 shrink-0 items-center justify-center overflow-hidden rounded-md border border-ink-200 bg-ink-50 text-ink-400">
                    {isImage(a) ? (
                      <span
                        role="img"
                        aria-label={`Preview of ${a.filename}`}
                        className="h-full w-full bg-cover bg-center"
                        style={{ backgroundImage: `url("${attachmentContentUrl(a.id)}")` }}
                      />
                    ) : (
                      <FileIcon size={20} />
                    )}
                  </span>
                  <div className="min-w-0 flex-1 basis-56">
                    <a
                      href={attachmentDownloadUrl(a.id)}
                      className="block truncate text-sm font-medium text-ink-900 hover:underline"
                    >
                      {a.filename}
                    </a>
                    <div className="mt-0.5 flex flex-wrap items-center gap-x-3 gap-y-0.5 text-xs text-ink-600">
                      <span>{a.content_type}</span>
                      <span>{formatBytes(a.size)}</span>
                      {a.uploader && (
                        <span className="flex items-center gap-1">
                          <Avatar name={a.uploader.display_name} seed={a.uploader.id} size={14} />
                          {a.uploader.display_name}
                        </span>
                      )}
                      <RelativeTime iso={a.created_at} />
                    </div>
                  </div>
                  <div className="flex shrink-0 flex-wrap items-center gap-1.5">
                    {isImage(a) && (
                      <>
                        <Button
                          size="sm"
                          onClick={() =>
                            copy(`![${a.filename}](${absoluteUrl(attachmentContentUrl(a.id))})`, "Markdown")
                          }
                        >
                          <Hash size={12} />
                          Markdown
                        </Button>
                        <Button
                          size="sm"
                          onClick={() =>
                            copy(
                              `<img src="${absoluteUrl(attachmentContentUrl(a.id))}" alt="${a.filename.replace(/"/g, "&quot;")}">`,
                              "HTML"
                            )
                          }
                        >
                          <Code2 size={12} />
                          HTML
                        </Button>
                      </>
                    )}
                    <a href={attachmentDownloadUrl(a.id)} className="btn btn-secondary btn-sm">
                      <Download size={12} />
                      Download
                    </a>
                    {writable && (
                      <Button
                        size="sm"
                        variant="danger"
                        aria-label={`Delete ${a.filename}`}
                        onClick={() => setToDelete(a)}
                      >
                        <Trash2 size={12} />
                        Delete
                      </Button>
                    )}
                  </div>
                </li>
              ))}
            </ul>
          )}
        </div>
      </div>

      {toDelete && (
        <ConfirmDialog
          title="Delete attachment"
          tone="danger"
          confirmLabel="Delete"
          busy={remove.isPending}
          onConfirm={() => remove.mutate(toDelete)}
          onCancel={() => setToDelete(null)}
          message={
            <>
              <strong>{toDelete.filename}</strong> will be deleted. Pages that embed it will show a broken image.
            </>
          }
        />
      )}
    </div>
  );
}
