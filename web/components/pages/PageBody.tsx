"use client";

import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { FileQuestion, Lock } from "lucide-react";
import { ApiError, getPage, type PageDetail } from "@/lib/api";
import { ContentView, useHeadings } from "@/components/editor/ContentView";
import { Button } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";
import { PageLabels } from "@/components/labels/PageLabels";
import { PageComments } from "@/components/comments/PageComments";
import { useSpace } from "@/components/spaces/SpaceContext";
import { PageBreadcrumbs } from "./Breadcrumbs";
import { ChildPages } from "./ChildPages";
import { PageActions } from "./PageActions";
import { PageHeader } from "./PageHeader";
import { PageSkeleton } from "./Skeletons";
import { TableOfContents } from "./TableOfContents";
import { pageEditHref, pageHref } from "./utils";

const PRINT_CSS = "@media print { body aside, body header.sticky { display: none !important; } }";

export function PageBody({ pageId, canonical = true }: { pageId: string; canonical?: boolean }) {
  const router = useRouter();
  const pathname = usePathname();
  const { space, isAdmin, canEdit, archived } = useSpace();
  const contentRef = useRef<HTMLDivElement>(null);
  const [activeComment, setActiveComment] = useState<string | null>(null);

  const query = useQuery<PageDetail, Error>({
    queryKey: ["page", pageId],
    queryFn: () => getPage(pageId),
    retry: false,
    staleTime: 0,
  });
  const page = query.data;
  const headings = useHeadings(page?.content_html ?? "");

  useEffect(() => {
    if (!canonical || !page) return;
    const target = pageHref(page.space_key ?? space.key, page);
    let current = pathname;
    try {
      current = decodeURI(pathname);
    } catch {
      current = pathname;
    }
    if (current !== target) {
      router.replace(`${target}${window.location.search}${window.location.hash}`);
    }
  }, [canonical, page, pathname, router, space.key]);

  useEffect(() => {
    if (!page) return;
    const previous = document.title;
    document.title = `${page.title || "Untitled"} | Knowledge`;
    return () => {
      document.title = previous;
    };
  }, [page]);

  const scrolledFor = useRef<string | null>(null);
  const pageIdLoaded = page?.id;
  useEffect(() => {
    if (!pageIdLoaded || scrolledFor.current === pageIdLoaded) return;
    scrolledFor.current = pageIdLoaded;
    const hash = window.location.hash;
    if (!hash || hash.startsWith("#comment-")) return;
    const timer = window.setTimeout(() => {
      try {
        document.getElementById(decodeURIComponent(hash.slice(1)))?.scrollIntoView();
      } catch {
        return;
      }
    }, 150);
    return () => window.clearTimeout(timer);
  }, [pageIdLoaded]);

  if (query.isPending) return <PageSkeleton />;

  if (query.isError || !page) {
    const status = query.error instanceof ApiError ? query.error.status : 0;
    return (
      <div className="mx-auto w-full max-w-xl px-4 py-16">
        {status === 403 ? (
          <EmptyState
            icon={<Lock size={18} />}
            title="You do not have access to this page"
            description="Ask a space admin for access."
          />
        ) : status === 404 ? (
          <EmptyState
            icon={<FileQuestion size={18} />}
            title="Page not found"
            description="It may have been moved to the trash or deleted, may be an unpublished draft, or you may not have access."
            action={
              <Link href={`/spaces/${space.key}`} className="btn btn-primary">
                Go to space home
              </Link>
            }
          />
        ) : (
          <EmptyState
            title="Could not load this page"
            description={query.error?.message}
            action={
              <Button variant="primary" onClick={() => void query.refetch()}>
                Try again
              </Button>
            }
          />
        )}
      </div>
    );
  }

  const trashed = page.status === "trashed";
  const hasContent = page.content_html.trim().length > 0;

  return (
    <div className="flex min-h-0 flex-1 bg-ink-0">
      <style>{PRINT_CSS}</style>
      <div className="min-w-0 flex-1 px-4 py-6 sm:px-8 lg:px-10 print:p-0">
        <div className="mx-auto w-full max-w-[860px]">
          <div className="mb-5 flex flex-wrap items-center justify-between gap-x-4 gap-y-3">
            <PageBreadcrumbs space={space} ancestors={page.breadcrumbs ?? []} />
            <PageActions page={page} />
          </div>

          <PageHeader page={page} spaceKey={space.key} canEdit={canEdit} />

          <div
            ref={contentRef}
            className="mt-6 [&_:is(h1,h2,h3,h4,h5,h6)]:scroll-mt-20"
          >
            {hasContent ? (
              <ContentView html={page.content_html} />
            ) : (
              <p className="rounded-md border border-dashed border-ink-200 px-4 py-8 text-center text-sm text-ink-600">
                This page has no content yet.
                {canEdit && !trashed && (
                  <>
                    {" "}
                    <Link href={pageEditHref(space.key, page.id)} className="link">
                      Start writing
                    </Link>
                  </>
                )}
              </p>
            )}
          </div>

          <div className="mt-8">
            <PageLabels pageId={page.id} canEdit={canEdit && !trashed} initial={page.labels} />
          </div>

          <ChildPages spaceKey={space.key} pages={page.children ?? []} />

          <div className="mt-10 print:hidden">
            <PageComments
              page={page}
              canComment={!archived}
              canEdit={isAdmin}
              contentRef={contentRef}
              activeCommentId={activeComment ?? undefined}
              onActiveCommentChange={setActiveComment}
            />
          </div>
        </div>
      </div>
      {headings.length > 0 && (
        <aside className="hidden w-64 shrink-0 xl:block print:hidden">
          <div className="sticky top-14 max-h-[calc(100vh-3.5rem)] overflow-y-auto py-8 pl-2 pr-6">
            <TableOfContents headings={headings} />
          </div>
        </aside>
      )}
    </div>
  );
}
