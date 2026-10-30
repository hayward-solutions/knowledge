"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { Suspense, useEffect, useState, type FormEvent } from "react";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { formatDistanceToNowStrict } from "date-fns";
import { ChevronLeft, ChevronRight, Search as SearchIcon, X } from "lucide-react";
import { AppShell } from "@/components/layout/AppShell";
import { LabelPicker } from "@/components/labels/LabelPicker";
import { AuthorFilter } from "@/components/search/AuthorFilter";
import { Avatar } from "@/components/ui/Avatar";
import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";
import { Select } from "@/components/ui/Select";
import { Spinner } from "@/components/ui/Spinner";
import { search } from "@/lib/api/search";
import { listSpaces } from "@/lib/api/spaces";
import type { SearchHit } from "@/lib/api/types";

const PAGE_LIMIT = 20;

interface Filters {
  q: string;
  space: string;
  label: string;
  author: string;
  page: number;
}

function safeSnippet(snippet: string): string {
  return snippet.replace(/<(?!\/?mark>)/gi, "&lt;");
}

function relative(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return formatDistanceToNowStrict(d, { addSuffix: true });
}

function toUrl(f: Filters): string {
  const sp = new URLSearchParams();
  if (f.q.trim()) sp.set("q", f.q.trim());
  if (f.space) sp.set("space", f.space);
  if (f.label) sp.set("label", f.label);
  if (f.author) sp.set("author", f.author);
  if (f.page > 1) sp.set("page", String(f.page));
  const s = sp.toString();
  return s ? `/search?${s}` : "/search";
}

function pageWindow(current: number, total: number): (number | "gap")[] {
  const pages = new Set<number>([1, total, current, current - 1, current + 1]);
  const sorted = Array.from(pages)
    .filter((p) => p >= 1 && p <= total)
    .sort((a, b) => a - b);
  const out: (number | "gap")[] = [];
  sorted.forEach((p, i) => {
    if (i > 0 && p - sorted[i - 1] > 1) out.push("gap");
    out.push(p);
  });
  return out;
}

function ResultItem({ hit }: { hit: SearchHit }) {
  const href = `/spaces/${hit.space_key}/pages/${hit.page.id}/${hit.page.slug}`;
  return (
    <li className="surface p-4">
      <div className="flex flex-wrap items-center gap-2">
        <Link
          href={href}
          className="min-w-0 break-words text-base font-semibold text-brand-700 hover:underline"
        >
          {hit.page.title || "Untitled"}
        </Link>
        <Badge tone="blue">{hit.space_key}</Badge>
        {hit.page.status === "draft" && <Badge tone="amber">Draft</Badge>}
      </div>
      {hit.snippet && (
        <p
          className="mt-1.5 break-words text-sm leading-relaxed text-ink-700 [&_mark]:rounded-sm [&_mark]:bg-warning-100 [&_mark]:px-0.5 [&_mark]:font-medium [&_mark]:text-ink-900"
          dangerouslySetInnerHTML={{ __html: safeSnippet(hit.snippet) }}
        />
      )}
      <div className="mt-2 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-ink-500">
        {hit.author && (
          <span className="inline-flex items-center gap-1.5">
            <Avatar name={hit.author.display_name} seed={hit.author.id} size={16} />
            {hit.author.display_name}
          </span>
        )}
        <time dateTime={hit.updated_at} title={new Date(hit.updated_at).toLocaleString()}>
          Updated {relative(hit.updated_at)}
        </time>
      </div>
    </li>
  );
}

function SearchView() {
  const router = useRouter();
  const params = useSearchParams();
  const q = params.get("q") ?? "";
  const space = params.get("space") ?? "";
  const label = params.get("label") ?? "";
  const author = params.get("author") ?? "";
  const page = Math.max(1, Number.parseInt(params.get("page") ?? "1", 10) || 1);
  const current: Filters = { q, space, label, author, page };

  const [draft, setDraft] = useState(q);
  useEffect(() => {
    setDraft(q);
  }, [q]);

  const spaces = useQuery({ queryKey: ["spaces"], queryFn: listSpaces });

  const hasQuery = q.trim() !== "";
  const results = useQuery({
    queryKey: ["search", q.trim(), space, label, author, page],
    queryFn: () =>
      search({
        q: q.trim(),
        space: space || undefined,
        label: label || undefined,
        author: author || undefined,
        page,
        limit: PAGE_LIMIT,
      }),
    enabled: hasQuery,
    placeholderData: keepPreviousData,
  });

  function go(next: Partial<Filters>) {
    router.push(toUrl({ ...current, page: 1, ...next }));
  }

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    go({ q: draft });
  }

  const data = results.data;
  const totalPages = data ? Math.max(1, Math.ceil(data.total / (data.limit || PAGE_LIMIT))) : 1;
  const filtered = Boolean(space || label || author);
  const first = data && data.total > 0 ? (data.page - 1) * data.limit + 1 : 0;
  const last = data ? Math.min(data.total, (data.page - 1) * data.limit + data.hits.length) : 0;

  return (
    <div className="mx-auto w-full max-w-4xl px-4 py-6 sm:px-8">
      <h1 className="mb-4 text-2xl font-semibold text-ink-900">Search</h1>

      <form role="search" onSubmit={onSubmit} className="flex gap-2">
        <div className="relative min-w-0 flex-1">
          <SearchIcon
            size={15}
            className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-ink-400"
          />
          <input
            type="search"
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            placeholder="Search pages"
            aria-label="Search query"
            className="control h-10 w-full pl-9 text-[15px]"
          />
        </div>
        <Button type="submit" variant="primary" className="h-10">
          Search
        </Button>
      </form>

      <div className="mt-3 flex flex-wrap items-center gap-2">
        <Select
          aria-label="Space"
          value={space}
          onChange={(e) => go({ space: e.target.value })}
          className="w-44 max-w-full"
        >
          <option value="">All spaces</option>
          {space && !spaces.data?.some((s) => s.key === space) && (
            <option value={space}>{space}</option>
          )}
          {(spaces.data ?? []).map((s) => (
            <option key={s.id} value={s.key}>
              {s.name} ({s.key})
            </option>
          ))}
        </Select>

        {label ? (
          <span className="inline-flex h-8 items-center gap-1 rounded-md border border-ink-200 bg-ink-0 pl-2.5 pr-1 text-sm text-ink-800">
            <span className="text-ink-500">Label:</span>
            <span className="max-w-[10rem] truncate font-medium">{label}</span>
            <button
              type="button"
              aria-label="Clear label filter"
              onClick={() => go({ label: "" })}
              className="flex h-5 w-5 items-center justify-center rounded-full text-ink-500 hover:bg-ink-100 hover:text-ink-900"
            >
              <X size={12} />
            </button>
          </span>
        ) : (
          <LabelPicker
            placeholder="Label"
            ariaLabel="Label filter"
            closeOnSelect
            onSelect={(name) => go({ label: name })}
          />
        )}

        <AuthorFilter value={author} onChange={(id) => go({ author: id })} />

        {filtered && (
          <Button
            variant="ghost"
            size="sm"
            onClick={() => go({ space: "", label: "", author: "" })}
          >
            Clear filters
          </Button>
        )}
      </div>

      <div className="mt-6" aria-live="polite">
        {!hasQuery && (
          <EmptyState
            icon={<SearchIcon size={18} />}
            title="Search Knowledge"
            description="Enter a search term to find pages by title or content. Use quotes for phrases and a minus sign to exclude words."
          />
        )}

        {hasQuery && results.isPending && (
          <div className="flex items-center gap-2 py-8 text-sm text-ink-500">
            <Spinner size={14} /> Searching
          </div>
        )}

        {hasQuery && results.isError && (
          <div className="rounded-md border border-danger-100 bg-danger-50 px-3 py-3 text-sm text-danger-700">
            <p>{results.error.message}</p>
            <button
              type="button"
              onClick={() => void results.refetch()}
              className="mt-2 text-xs font-medium underline"
            >
              Try again
            </button>
          </div>
        )}

        {hasQuery && data && data.total === 0 && (
          <EmptyState
            icon={<SearchIcon size={18} />}
            title="No results"
            description={
              filtered
                ? `Nothing matched "${q.trim()}" with these filters. Try removing a filter or using different words.`
                : `Nothing matched "${q.trim()}". Check the spelling or try different words.`
            }
            action={
              filtered ? (
                <Button onClick={() => go({ space: "", label: "", author: "" })}>
                  Clear filters
                </Button>
              ) : undefined
            }
          />
        )}

        {hasQuery && data && data.total > 0 && (
          <>
            <p className="mb-3 flex items-center gap-2 text-sm text-ink-600">
              <span>
                {first}-{last} of {data.total} {data.total === 1 ? "result" : "results"}
              </span>
              {results.isFetching && <Spinner size={12} />}
            </p>
            <ul className="space-y-3">
              {data.hits.map((hit) => (
                <ResultItem key={hit.page.id} hit={hit} />
              ))}
            </ul>

            {totalPages > 1 && (
              <nav aria-label="Pagination" className="mt-6 flex flex-wrap items-center justify-center gap-1">
                <Button
                  size="sm"
                  disabled={data.page <= 1}
                  onClick={() => router.push(toUrl({ ...current, page: data.page - 1 }))}
                  aria-label="Previous page"
                >
                  <ChevronLeft size={14} />
                  <span className="hidden sm:inline">Previous</span>
                </Button>
                {pageWindow(data.page, totalPages).map((p, i) =>
                  p === "gap" ? (
                    <span key={`gap-${i}`} className="px-1 text-ink-400">
                      ...
                    </span>
                  ) : (
                    <Button
                      key={p}
                      size="sm"
                      variant={p === data.page ? "primary" : "secondary"}
                      aria-current={p === data.page ? "page" : undefined}
                      onClick={() => router.push(toUrl({ ...current, page: p }))}
                    >
                      {p}
                    </Button>
                  )
                )}
                <Button
                  size="sm"
                  disabled={data.page >= totalPages}
                  onClick={() => router.push(toUrl({ ...current, page: data.page + 1 }))}
                  aria-label="Next page"
                >
                  <span className="hidden sm:inline">Next</span>
                  <ChevronRight size={14} />
                </Button>
              </nav>
            )}
          </>
        )}
      </div>
    </div>
  );
}

export default function SearchPage() {
  return (
    <AppShell>
      <Suspense
        fallback={
          <div className="flex items-center gap-2 px-8 py-8 text-sm text-ink-500">
            <Spinner size={14} /> Loading
          </div>
        }
      >
        <SearchView />
      </Suspense>
    </AppShell>
  );
}
