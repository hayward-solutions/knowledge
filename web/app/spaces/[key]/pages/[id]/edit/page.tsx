"use client";

import { useParams, useRouter } from "next/navigation";
import { useCallback, useEffect, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { format, formatDistanceToNow } from "date-fns";
import { AlertTriangle, Check, Info, Loader2, Users, X } from "lucide-react";
import {
  ApiError,
  attachmentContentPath,
  discardDraft,
  getDraft,
  getPage,
  isVersionConflict,
  publishPage,
  saveDraft,
  sendEditingHeartbeat,
  uploadAttachment,
  type PageDetail,
  type PageDraft,
  type UserRef,
} from "@/lib/api";
import { useMe } from "@/lib/auth";
import { Editor } from "@/components/editor/Editor";
import type { EditorChange, EditorHandle, UploadedImage } from "@/components/editor/types";
import { Button } from "@/components/ui/Button";
import { ConfirmDialog } from "@/components/ui/ConfirmDialog";
import { EmptyState } from "@/components/ui/EmptyState";
import { Field } from "@/components/ui/Field";
import { Input } from "@/components/ui/Input";
import { Modal } from "@/components/ui/Modal";
import { Spinner } from "@/components/ui/Spinner";
import { useToast } from "@/components/ui/Toast";

const AUTOSAVE_MS = 1500;
const HEARTBEAT_MS = 15000;

type SaveStatus = "idle" | "unsaved" | "saving" | "saved" | "error";

async function loadDraft(pageId: string): Promise<PageDraft | null> {
  try {
    return await getDraft(pageId);
  } catch (err) {
    if (err instanceof ApiError && err.status === 404) return null;
    throw err;
  }
}

function messageOf(err: unknown, fallback: string): string {
  return err instanceof Error && err.message ? err.message : fallback;
}

function joinNames(users: UserRef[]): string {
  const names = users.map((u) => u.display_name);
  if (names.length <= 1) return names[0] ?? "";
  if (names.length === 2) return `${names[0]} and ${names[1]}`;
  return `${names.slice(0, -1).join(", ")} and ${names[names.length - 1]}`;
}

export default function EditPage() {
  const params = useParams<{ key: string; id: string }>();
  return <EditLoader spaceKey={String(params.key)} pageId={String(params.id)} />;
}

function Centered({ children }: { children: React.ReactNode }) {
  return <div className="mx-auto w-full max-w-xl px-4 py-16">{children}</div>;
}

function EditLoader({ spaceKey, pageId }: { spaceKey: string; pageId: string }) {
  const router = useRouter();
  const pageQuery = useQuery<PageDetail, Error>({
    queryKey: ["editor", "page", pageId],
    queryFn: () => getPage(pageId),
    retry: false,
    staleTime: Infinity,
    gcTime: 0,
    refetchOnMount: "always",
  });
  const draftQuery = useQuery<PageDraft | null, Error>({
    queryKey: ["editor", "draft", pageId],
    queryFn: () => loadDraft(pageId),
    enabled: pageQuery.isSuccess,
    retry: false,
    staleTime: Infinity,
    gcTime: 0,
    refetchOnMount: "always",
  });

  const viewHref = `/spaces/${spaceKey}/pages/${pageId}`;

  if (pageQuery.isPending || (pageQuery.isSuccess && draftQuery.isPending)) {
    return (
      <Centered>
        <div className="flex justify-center">
          <Spinner size={24} label="Loading editor" />
        </div>
      </Centered>
    );
  }

  const failure = pageQuery.error ?? draftQuery.error;
  if (failure) {
    const status = failure instanceof ApiError ? failure.status : 0;
    if (status === 403) {
      return (
        <Centered>
          <EmptyState
            title="You can't edit this page"
            description="You need editor access to this space to change its pages."
            action={
              <Button variant="secondary" onClick={() => router.push(viewHref)}>
                Back to page
              </Button>
            }
          />
        </Centered>
      );
    }
    if (status === 404) {
      return (
        <Centered>
          <EmptyState
            title="Page not found"
            description="It may have been deleted, or you may not have access to it."
            action={
              <Button variant="secondary" onClick={() => router.push(`/spaces/${spaceKey}`)}>
                Back to space
              </Button>
            }
          />
        </Centered>
      );
    }
    return (
      <Centered>
        <EmptyState
          title="Could not load the editor"
          description={failure.message}
          action={
            <Button
              variant="primary"
              onClick={() => {
                pageQuery.refetch();
                draftQuery.refetch();
              }}
            >
              Try again
            </Button>
          }
        />
      </Centered>
    );
  }

  const page = pageQuery.data;
  if (!page) return null;

  if (page.status === "trashed") {
    return (
      <Centered>
        <EmptyState
          title="This page is in the trash"
          description="Restore it before editing."
          action={
            <Button variant="secondary" onClick={() => router.push(`/spaces/${spaceKey}/trash`)}>
              Open trash
            </Button>
          }
        />
      </Centered>
    );
  }

  if (page.my_role === "viewer") {
    return (
      <Centered>
        <EmptyState
          title="You can't edit this page"
          description="You have view-only access to this space."
          action={
            <Button variant="secondary" onClick={() => router.push(viewHref)}>
              Back to page
            </Button>
          }
        />
      </Centered>
    );
  }

  return <EditSession spaceKey={spaceKey} page={page} draft={draftQuery.data ?? null} />;
}

function SaveIndicator({
  status,
  savedAt,
  error,
  onRetry,
}: {
  status: SaveStatus;
  savedAt: Date | null;
  error: string | null;
  onRetry: () => void;
}) {
  if (status === "saving") {
    return (
      <span className="inline-flex items-center gap-1.5 text-xs text-ink-600" role="status">
        <Loader2 size={13} className="animate-spin" />
        Saving...
      </span>
    );
  }
  if (status === "error") {
    return (
      <span
        className="inline-flex items-center gap-1.5 text-xs text-danger-600"
        role="alert"
        title={error ?? undefined}
      >
        <AlertTriangle size={13} />
        Save failed
        <button type="button" onClick={onRetry} className="font-semibold underline">
          Retry
        </button>
      </span>
    );
  }
  if (status === "unsaved") {
    return (
      <span className="text-xs text-ink-500" role="status">
        Unsaved changes
      </span>
    );
  }
  if (status === "saved" && savedAt) {
    return (
      <span className="inline-flex items-center gap-1.5 text-xs text-ink-600" role="status">
        <Check size={13} className="text-success-600" />
        Saved {format(savedAt, "HH:mm")}
      </span>
    );
  }
  return null;
}

function EditSession({
  spaceKey,
  page,
  draft,
}: {
  spaceKey: string;
  page: PageDetail;
  draft: PageDraft | null;
}) {
  const router = useRouter();
  const qc = useQueryClient();
  const toast = useToast();
  const { data: me } = useMe();
  const pageId = page.id;

  const source = draft ?? page;
  const editorRef = useRef<EditorHandle>(null);

  const [title, setTitle] = useState(source.title);
  const titleRef = useRef(source.title);
  const valueRef = useRef<{ json: EditorChange["json"] | null; html: string }>({
    json: source.content_json,
    html: source.content_html,
  });
  const baseRef = useRef(draft ? draft.base_version : page.version);
  const dirtyRef = useRef(false);
  const revisionRef = useRef(0);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const inflightRef = useRef<Promise<boolean> | null>(null);
  const saveRef = useRef<() => Promise<boolean>>(async () => true);

  const [status, setStatus] = useState<SaveStatus>("idle");
  const [savedAt, setSavedAt] = useState<Date | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);

  const [resumed, setResumed] = useState<PageDraft | null>(draft);
  const [bannerDismissed, setBannerDismissed] = useState(false);
  const [confirmDiscard, setConfirmDiscard] = useState(false);
  const [discarding, setDiscarding] = useState(false);

  const [editors, setEditors] = useState<UserRef[]>(page.editors ?? []);

  const [publishOpen, setPublishOpen] = useState(false);
  const [publishMessage, setPublishMessage] = useState("");
  const [publishing, setPublishing] = useState(false);
  const [conflictVersion, setConflictVersion] = useState<number | null>(null);
  const [leaveConfirm, setLeaveConfirm] = useState(false);

  const viewHref = `/spaces/${spaceKey}/pages/${pageId}/${page.slug}`;
  const isNew = page.status === "draft";

  const clearTimer = useCallback(() => {
    if (timerRef.current) {
      clearTimeout(timerRef.current);
      timerRef.current = null;
    }
  }, []);

  const saveNow = useCallback((): Promise<boolean> => {
    clearTimer();
    if (inflightRef.current) {
      return inflightRef.current.then(() => saveRef.current());
    }
    if (!dirtyRef.current) return Promise.resolve(true);
    const revision = revisionRef.current;
    setStatus("saving");
    const request = saveDraft(pageId, {
      title: titleRef.current.trim() || page.title || "Untitled",
      content_json: valueRef.current.json,
      content_html: valueRef.current.html,
      base_version: baseRef.current,
    })
      .then(() => {
        if (revisionRef.current === revision) {
          dirtyRef.current = false;
          setStatus("saved");
          setSavedAt(new Date());
        } else {
          setStatus("unsaved");
        }
        setSaveError(null);
        return true;
      })
      .catch((err: unknown) => {
        setStatus("error");
        setSaveError(messageOf(err, "Could not save draft"));
        return false;
      })
      .finally(() => {
        inflightRef.current = null;
      });
    inflightRef.current = request;
    return request;
  }, [clearTimer, page.title, pageId]);

  useEffect(() => {
    saveRef.current = saveNow;
  }, [saveNow]);

  const markDirty = useCallback(() => {
    dirtyRef.current = true;
    revisionRef.current += 1;
    setStatus((s) => (s === "saving" ? s : "unsaved"));
    clearTimer();
    timerRef.current = setTimeout(() => {
      timerRef.current = null;
      void saveRef.current();
    }, AUTOSAVE_MS);
  }, [clearTimer]);

  const onEditorChange = useCallback(
    (change: EditorChange) => {
      valueRef.current = { json: change.json, html: change.html };
      markDirty();
    },
    [markDirty]
  );

  const onTitleChange = useCallback(
    (value: string) => {
      setTitle(value);
      titleRef.current = value;
      markDirty();
    },
    [markDirty]
  );

  useEffect(() => {
    const onBeforeUnload = (event: BeforeUnloadEvent) => {
      if (!dirtyRef.current) return;
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", onBeforeUnload);
    return () => window.removeEventListener("beforeunload", onBeforeUnload);
  }, []);

  useEffect(() => {
    return () => {
      if (timerRef.current) clearTimeout(timerRef.current);
      if (dirtyRef.current) void saveRef.current();
    };
  }, []);

  useEffect(() => {
    let cancelled = false;
    const beat = async () => {
      try {
        const state = await sendEditingHeartbeat(pageId);
        if (!cancelled) setEditors(state.editors ?? []);
      } catch {
        return;
      }
    };
    void beat();
    const interval = setInterval(beat, HEARTBEAT_MS);
    return () => {
      cancelled = true;
      clearInterval(interval);
    };
  }, [pageId]);

  const uploadImage = useCallback(
    async (file: File): Promise<UploadedImage> => {
      const attachment = await uploadAttachment(pageId, file);
      return { id: attachment.id, url: attachmentContentPath(attachment.id) };
    },
    [pageId]
  );

  const flushSave = useCallback(async (): Promise<boolean> => {
    clearTimer();
    while (dirtyRef.current) {
      const ok = await saveRef.current();
      if (!ok) return false;
    }
    return true;
  }, [clearTimer]);

  async function close() {
    const ok = await flushSave();
    if (!ok) {
      setLeaveConfirm(true);
      return;
    }
    router.push(viewHref);
  }

  async function discard() {
    setDiscarding(true);
    try {
      clearTimer();
      if (inflightRef.current) await inflightRef.current;
      await discardDraft(pageId);
      editorRef.current?.setContent({ json: page.content_json, html: page.content_html });
      valueRef.current = { json: page.content_json, html: page.content_html };
      titleRef.current = page.title;
      setTitle(page.title);
      baseRef.current = page.version;
      dirtyRef.current = false;
      revisionRef.current += 1;
      setResumed(null);
      setStatus("idle");
      setSaveError(null);
      setConfirmDiscard(false);
      toast.success("Draft discarded");
    } catch (err) {
      toast.error(messageOf(err, "Could not discard the draft"));
    } finally {
      setDiscarding(false);
    }
  }

  async function publish(force: boolean) {
    const finalTitle = titleRef.current.trim();
    if (!finalTitle) return;
    setPublishing(true);
    clearTimer();
    try {
      if (inflightRef.current) await inflightRef.current;
      const live = editorRef.current?.getValue();
      const content = live
        ? { json: live.json, html: live.html }
        : { json: valueRef.current.json, html: valueRef.current.html };
      const message = publishMessage.trim();
      const published = await publishPage(pageId, {
        title: finalTitle,
        content_json: content.json,
        content_html: content.html,
        base_version: baseRef.current,
        message: message || undefined,
        force: force || undefined,
      });
      dirtyRef.current = false;
      setPublishOpen(false);
      setConflictVersion(null);
      void qc.invalidateQueries({ predicate: (q) => q.queryKey[0] !== "editor" });
      toast.success(isNew ? "Page published" : "Changes published");
      router.push(`/spaces/${spaceKey}/pages/${pageId}/${published.slug || page.slug}`);
    } catch (err) {
      if (isVersionConflict(err)) {
        setPublishOpen(false);
        setConflictVersion(err.body?.current_version ?? page.version + 1);
      } else {
        toast.error(messageOf(err, "Could not publish"));
      }
    } finally {
      setPublishing(false);
    }
  }

  async function reloadLatest() {
    await flushSave();
    router.push(viewHref);
  }

  const others = editors.filter((e) => e.id !== me?.id);
  const staleDraft = resumed !== null && resumed.base_version < page.version;
  const showResume = resumed !== null && !bannerDismissed;

  return (
    <div
      className="flex min-h-0 flex-1 flex-col"
      style={{ ["--kn-toolbar-top" as string]: "105px" }}
    >
      <div className="sticky top-14 z-20 flex h-[49px] items-center gap-3 border-b border-ink-200 bg-ink-0/95 px-4 backdrop-blur sm:px-6">
        <div className="min-w-0 flex-1 truncate text-sm text-ink-600">
          {isNew ? "New page" : "Editing"}
          <span className="hidden sm:inline">
            {" - "}
            <span className="font-medium text-ink-900">{title.trim() || "Untitled"}</span>
          </span>
        </div>
        <SaveIndicator
          status={status}
          savedAt={savedAt}
          error={saveError}
          onRetry={() => void saveNow()}
        />
        <Button variant="ghost" onClick={() => void close()}>
          Close
        </Button>
        <Button
          variant="primary"
          disabled={!title.trim() || publishing}
          onClick={() => {
            clearTimer();
            setPublishOpen(true);
          }}
        >
          Publish
        </Button>
      </div>

      <div className="mx-auto w-full max-w-[940px] px-4 pb-16 pt-6 sm:px-6">
        {showResume && resumed && (
          <div
            role="status"
            className="mb-4 flex flex-wrap items-center gap-x-3 gap-y-2 rounded-md border border-brand-100 bg-brand-50 px-3 py-2 text-sm text-ink-800"
          >
            <Info size={16} className="shrink-0 text-brand-600" />
            <span className="min-w-0 flex-1">
              Resuming your unpublished changes from{" "}
              {formatDistanceToNow(new Date(resumed.updated_at), { addSuffix: true })}
              {staleDraft && (
                <>
                  . This page has been published since (now version {page.version}), so publishing
                  may need conflict resolution.
                </>
              )}
            </span>
            <Button size="sm" variant="secondary" onClick={() => setConfirmDiscard(true)}>
              Discard draft
            </Button>
            <button
              type="button"
              aria-label="Dismiss"
              onClick={() => setBannerDismissed(true)}
              className="rounded-xs p-1 text-ink-500 hover:bg-ink-100 hover:text-ink-900"
            >
              <X size={14} />
            </button>
          </div>
        )}

        {others.length > 0 && (
          <div
            role="status"
            className="mb-4 flex items-center gap-2 rounded-md border border-warning-100 bg-warning-50 px-3 py-2 text-sm text-ink-800"
          >
            <Users size={16} className="shrink-0 text-warning-600" />
            <span>
              {joinNames(others)} {others.length === 1 ? "is" : "are"} also editing this page. The
              last person to publish wins unless a conflict is detected.
            </span>
          </div>
        )}

        <input
          value={title}
          onChange={(e) => onTitleChange(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              editorRef.current?.focus("start");
            }
          }}
          placeholder="Page title"
          aria-label="Page title"
          maxLength={255}
          autoFocus={isNew && !source.title.trim()}
          className="mb-4 w-full border-0 bg-transparent px-0 text-3xl font-bold tracking-tight text-ink-900 outline-none placeholder:text-ink-300 focus-visible:outline-none sm:text-4xl"
        />

        <Editor
          ref={editorRef}
          value={{ json: source.content_json, html: source.content_html }}
          onChange={onEditorChange}
          editable
          variant="full"
          uploadImage={uploadImage}
        />
      </div>

      {publishOpen && (
        <Modal
          title={isNew ? "Publish page" : "Publish changes"}
          onClose={() => !publishing && setPublishOpen(false)}
          width={480}
          footer={
            <>
              <Button variant="ghost" onClick={() => setPublishOpen(false)} disabled={publishing}>
                Cancel
              </Button>
              <Button variant="primary" onClick={() => void publish(false)} disabled={publishing}>
                {publishing ? "Publishing..." : "Publish"}
              </Button>
            </>
          }
        >
          <Field
            label="Version message (optional)"
            hint="Shown in the page history next to this version."
          >
            <Input
              data-autofocus
              value={publishMessage}
              maxLength={500}
              placeholder="What changed?"
              onChange={(e) => setPublishMessage(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && !publishing) {
                  e.preventDefault();
                  void publish(false);
                }
              }}
            />
          </Field>
        </Modal>
      )}

      {conflictVersion !== null && (
        <Modal
          title="This page was published by someone else"
          onClose={() => !publishing && setConflictVersion(null)}
          width={520}
          footer={
            <>
              <Button
                variant="ghost"
                onClick={() => setConflictVersion(null)}
                disabled={publishing}
              >
                Keep editing
              </Button>
              <Button variant="secondary" onClick={() => void reloadLatest()} disabled={publishing}>
                Reload latest
              </Button>
              <Button variant="danger" onClick={() => void publish(true)} disabled={publishing}>
                {publishing ? "Publishing..." : "Publish anyway"}
              </Button>
            </>
          }
        >
          <div className="space-y-2 text-sm text-ink-700">
            <p>
              Version {conflictVersion} of this page was published while you were editing from
              version {baseRef.current}.
            </p>
            <p>
              <strong>Reload latest</strong> takes you to the current page and keeps your draft so
              you can copy anything you need. <strong>Publish anyway</strong> replaces the latest
              version with your content; the other version stays in the page history.
            </p>
          </div>
        </Modal>
      )}

      {confirmDiscard && (
        <ConfirmDialog
          title="Discard draft?"
          message="Your unpublished changes will be deleted and the editor will go back to the published page."
          confirmLabel="Discard draft"
          tone="danger"
          busy={discarding}
          onConfirm={() => void discard()}
          onCancel={() => !discarding && setConfirmDiscard(false)}
        />
      )}

      {leaveConfirm && (
        <ConfirmDialog
          title="Changes could not be saved"
          message="Your latest edits were not saved to your draft. Leave anyway and lose them?"
          confirmLabel="Leave without saving"
          tone="danger"
          onConfirm={() => {
            dirtyRef.current = false;
            router.push(viewHref);
          }}
          onCancel={() => setLeaveConfirm(false)}
        />
      )}
    </div>
  );
}
