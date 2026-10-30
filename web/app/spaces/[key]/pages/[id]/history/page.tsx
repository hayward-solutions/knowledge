"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, History, RotateCcw } from "lucide-react";
import {
  getPage,
  getVersion,
  listVersions,
  restoreVersion,
  type PageDetail,
  type PageVersion,
  type PageVersionSummary,
} from "@/lib/api";
import { cn } from "@/lib/cn";
import { ContentView } from "@/components/editor/ContentView";
import { Avatar } from "@/components/ui/Avatar";
import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { ConfirmDialog } from "@/components/ui/ConfirmDialog";
import { EmptyState } from "@/components/ui/EmptyState";
import { Spinner } from "@/components/ui/Spinner";
import { useToast } from "@/components/ui/Toast";
import { RelativeTime } from "@/components/pages/RelativeTime";
import { ListSkeleton } from "@/components/pages/Skeletons";
import { VersionDiff } from "@/components/pages/VersionDiff";
import { PageBreadcrumbs } from "@/components/pages/Breadcrumbs";
import { errorMessage, htmlToText, pageHref } from "@/components/pages/utils";
import { useSpace } from "@/components/spaces/SpaceContext";

type Mode = "changes" | "side" | "view";

function useVersionContent(pageId: string, version: number | null) {
  return useQuery<PageVersion, Error>({
    queryKey: ["versions", pageId, "content", version],
    queryFn: () => getVersion(pageId, version as number),
    enabled: version !== null,
    staleTime: Infinity,
  });
}

function authorName(v: PageVersionSummary): string {
  return v.author?.display_name ?? "Unknown";
}

function VersionPane({ version, loading }: { version: PageVersion | undefined; loading: boolean }) {
  if (loading || !version) {
    return (
      <div className="flex items-center justify-center py-12">
        <Spinner size={20} label="Loading version" />
      </div>
    );
  }
  return (
    <div className="min-w-0">
      <h3 className="mb-2 text-lg font-semibold text-ink-900">{version.title || "Untitled"}</h3>
      {version.content_html.trim() ? (
        <ContentView html={version.content_html} />
      ) : (
        <p className="text-sm text-ink-600">This version has no content.</p>
      )}
    </div>
  );
}

export default function HistoryPage() {
  const params = useParams<{ id: string }>();
  const id = String(params.id);
  const router = useRouter();
  const qc = useQueryClient();
  const toast = useToast();
  const { space, canEdit, refresh } = useSpace();

  const pageQuery = useQuery<PageDetail, Error>({
    queryKey: ["page", id],
    queryFn: () => getPage(id),
    retry: false,
  });
  const versionsQuery = useQuery<PageVersionSummary[], Error>({
    queryKey: ["versions", id],
    queryFn: () => listVersions(id),
    retry: false,
  });

  const versions = useMemo(
    () => [...(versionsQuery.data ?? [])].sort((a, b) => b.version - a.version),
    [versionsQuery.data]
  );

  const [selected, setSelected] = useState<number[]>([]);
  const [mode, setMode] = useState<Mode>("changes");
  const [viewing, setViewing] = useState<number | null>(null);
  const [restoreTarget, setRestoreTarget] = useState<number | null>(null);
  const initialised = useRef(false);

  useEffect(() => {
    if (initialised.current || versions.length === 0) return;
    initialised.current = true;
    setSelected(versions.length >= 2 ? [versions[0].version, versions[1].version] : [versions[0].version]);
  }, [versions]);

  const sortedSelection = useMemo(() => [...selected].sort((a, b) => a - b), [selected]);
  const newerNum = sortedSelection.length > 0 ? sortedSelection[sortedSelection.length - 1] : null;
  const olderNum = useMemo(() => {
    if (sortedSelection.length === 2) return sortedSelection[0];
    if (newerNum === null) return null;
    const lower = versions.find((v) => v.version < newerNum);
    return lower ? lower.version : null;
  }, [sortedSelection, newerNum, versions]);

  const viewNum = viewing ?? newerNum;
  const olderQuery = useVersionContent(id, mode === "view" ? null : olderNum);
  const newerQuery = useVersionContent(id, mode === "view" ? null : newerNum);
  const viewQuery = useVersionContent(id, mode === "view" ? viewNum : null);

  const oldText = useMemo(
    () => (olderNum === null ? "" : `${olderQuery.data?.title ?? ""}\n\n${htmlToText(olderQuery.data?.content_html ?? "")}`),
    [olderNum, olderQuery.data]
  );
  const newText = useMemo(
    () => `${newerQuery.data?.title ?? ""}\n\n${htmlToText(newerQuery.data?.content_html ?? "")}`,
    [newerQuery.data]
  );

  const restore = useMutation({
    mutationFn: (n: number) => restoreVersion(id, n),
    onSuccess: async (page, n) => {
      toast.success(`Restored version ${n}`);
      setRestoreTarget(null);
      await Promise.all([
        qc.invalidateQueries({ queryKey: ["page"] }),
        qc.invalidateQueries({ queryKey: ["versions", id] }),
        qc.invalidateQueries({ queryKey: ["dashboard"] }),
        refresh(),
      ]);
      router.push(pageHref(space.key, { id, slug: page.slug }));
    },
    onError: (err) => {
      setRestoreTarget(null);
      toast.error(errorMessage(err, "Could not restore version"));
    },
  });

  function toggle(n: number) {
    setSelected((cur) => {
      if (cur.includes(n)) return cur.filter((x) => x !== n);
      if (cur.length >= 2) return [cur[1], n];
      return [...cur, n];
    });
    if (mode === "view") setMode("changes");
  }

  const page = pageQuery.data;
  const currentVersion = page?.version ?? versions[0]?.version ?? 0;

  if (versionsQuery.isPending) {
    return (
      <div className="mx-auto w-full max-w-5xl px-4 py-8 sm:px-8">
        <ListSkeleton rows={6} />
      </div>
    );
  }

  if (versionsQuery.isError) {
    return (
      <div className="mx-auto w-full max-w-xl px-4 py-16">
        <EmptyState
          title="Could not load version history"
          description={versionsQuery.error.message}
          action={
            <Button variant="primary" onClick={() => void versionsQuery.refetch()}>
              Try again
            </Button>
          }
        />
      </div>
    );
  }

  return (
    <div className="flex-1 bg-ink-0 px-4 py-6 sm:px-8">
      <div className="mx-auto w-full max-w-6xl">
        <PageBreadcrumbs space={space} ancestors={page?.breadcrumbs ?? []} title={page?.title} />
        <div className="mt-4 flex flex-wrap items-center justify-between gap-3">
          <div className="flex min-w-0 items-center gap-2">
            <History size={20} className="shrink-0 text-ink-500" />
            <h1 className="truncate text-[22px] font-semibold tracking-tight text-ink-900">
              Version history
            </h1>
          </div>
          <Link
            href={page ? pageHref(space.key, page) : `/spaces/${space.key}/pages/${id}`}
            className="btn btn-secondary"
          >
            <ArrowLeft size={14} />
            Back to page
          </Link>
        </div>

        {versions.length === 0 ? (
          <div className="mt-8">
            <EmptyState
              title="No published versions yet"
              description="Versions appear here after the page is published."
            />
          </div>
        ) : (
          <div className="mt-6 grid gap-6 lg:grid-cols-[320px_minmax(0,1fr)]">
            <section aria-label="Versions">
              <p className="mb-2 text-xs text-ink-600">
                Select two versions to compare. With one selected, it is compared to the version before it.
              </p>
              <ul className="surface divide-y divide-ink-100 overflow-hidden">
                {versions.map((v) => {
                  const checked = selected.includes(v.version);
                  const isCurrent = v.version === currentVersion;
                  return (
                    <li
                      key={v.version}
                      className={cn("flex items-start gap-2 px-3 py-2.5", checked && "bg-brand-50")}
                    >
                      <input
                        type="checkbox"
                        checked={checked}
                        onChange={() => toggle(v.version)}
                        aria-label={`Compare version ${v.version}`}
                        className="mt-1"
                      />
                      <div className="min-w-0 flex-1">
                        <div className="flex flex-wrap items-center gap-1.5">
                          <span className="text-sm font-semibold text-ink-900">Version {v.version}</span>
                          {isCurrent && <Badge tone="green">Current</Badge>}
                        </div>
                        <div className="mt-0.5 flex items-center gap-1.5 text-xs text-ink-600">
                          <Avatar name={authorName(v)} seed={v.author_id} size={16} />
                          <span className="truncate">{authorName(v)}</span>
                          <span aria-hidden>·</span>
                          <RelativeTime iso={v.created_at} className="shrink-0" />
                        </div>
                        {v.message && (
                          <p className="mt-1 break-words text-xs italic text-ink-700">{v.message}</p>
                        )}
                        <div className="mt-1.5 flex items-center gap-2">
                          <button
                            type="button"
                            className="link text-xs"
                            onClick={() => {
                              setViewing(v.version);
                              setMode("view");
                            }}
                          >
                            View
                          </button>
                          {canEdit && !isCurrent && (
                            <button
                              type="button"
                              className="link inline-flex items-center gap-1 text-xs"
                              onClick={() => setRestoreTarget(v.version)}
                            >
                              <RotateCcw size={11} />
                              Restore
                            </button>
                          )}
                        </div>
                      </div>
                    </li>
                  );
                })}
              </ul>
            </section>

            <section aria-label="Comparison" className="min-w-0">
              <div role="tablist" className="mb-4 inline-flex rounded-md border border-ink-200 p-0.5">
                {(
                  [
                    ["changes", "Changes"],
                    ["side", "Side by side"],
                    ["view", "Single version"],
                  ] as const
                ).map(([value, label]) => (
                  <button
                    key={value}
                    type="button"
                    role="tab"
                    aria-selected={mode === value}
                    onClick={() => setMode(value)}
                    className={cn(
                      "rounded px-3 py-1 text-sm font-medium transition",
                      mode === value ? "bg-brand-50 text-brand-700" : "text-ink-600 hover:text-ink-900"
                    )}
                  >
                    {label}
                  </button>
                ))}
              </div>

              {mode === "changes" && (
                <>
                  <p className="mb-3 text-sm text-ink-700">
                    {olderNum === null ? (
                      <>Version {newerNum} (first version)</>
                    ) : (
                      <>
                        Comparing <strong>version {olderNum}</strong> to <strong>version {newerNum}</strong>
                      </>
                    )}
                  </p>
                  {newerQuery.isPending || (olderNum !== null && olderQuery.isPending) ? (
                    <div className="flex items-center justify-center py-12">
                      <Spinner size={20} label="Loading versions" />
                    </div>
                  ) : newerQuery.isError || olderQuery.isError ? (
                    <EmptyState title="Could not load these versions" />
                  ) : (
                    <VersionDiff oldText={oldText} newText={newText} />
                  )}
                </>
              )}

              {mode === "side" && (
                <div className="grid gap-4 md:grid-cols-2">
                  <div className="surface min-w-0 p-4">
                    <div className="mb-2 text-xs font-semibold uppercase tracking-wide text-ink-500">
                      {olderNum === null ? "Before" : `Version ${olderNum}`}
                    </div>
                    {olderNum === null ? (
                      <p className="text-sm text-ink-600">No earlier version.</p>
                    ) : (
                      <VersionPane version={olderQuery.data} loading={olderQuery.isPending} />
                    )}
                  </div>
                  <div className="surface min-w-0 p-4">
                    <div className="mb-2 text-xs font-semibold uppercase tracking-wide text-ink-500">
                      Version {newerNum}
                    </div>
                    <VersionPane version={newerQuery.data} loading={newerQuery.isPending} />
                  </div>
                </div>
              )}

              {mode === "view" && viewNum !== null && (
                <div className="surface p-5">
                  <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
                    <span className="text-xs font-semibold uppercase tracking-wide text-ink-500">
                      Version {viewNum}
                    </span>
                    {canEdit && viewNum !== currentVersion && (
                      <Button size="sm" onClick={() => setRestoreTarget(viewNum)}>
                        <RotateCcw size={12} />
                        Restore this version
                      </Button>
                    )}
                  </div>
                  <VersionPane version={viewQuery.data} loading={viewQuery.isPending} />
                </div>
              )}
            </section>
          </div>
        )}
      </div>

      {restoreTarget !== null && (
        <ConfirmDialog
          title="Restore this version"
          confirmLabel="Restore"
          busy={restore.isPending}
          onConfirm={() => restore.mutate(restoreTarget)}
          onCancel={() => setRestoreTarget(null)}
          message={
            <>
              Version {restoreTarget} will be published as a new version of this page. Existing versions are kept.
            </>
          }
        />
      )}
    </div>
  );
}
