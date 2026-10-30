"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { Tag } from "lucide-react";
import { listSpaceLabels } from "@/lib/api/labels";
import { EmptyState } from "@/components/ui/EmptyState";
import { Spinner } from "@/components/ui/Spinner";

export default function SpaceLabelsPage() {
  const params = useParams<{ key: string }>();
  const key = params.key;

  const { data, isPending, error, refetch } = useQuery({
    queryKey: ["space-labels", key],
    queryFn: () => listSpaceLabels(key),
  });

  return (
    <div className="mx-auto w-full max-w-4xl px-4 py-6 sm:px-8">
      <nav aria-label="Breadcrumb" className="mb-3 text-sm text-ink-600">
        <Link href={`/spaces/${key}`} className="hover:text-ink-900 hover:underline">
          {key}
        </Link>
        <span className="mx-1.5 text-ink-400">/</span>
        <span className="text-ink-900">Labels</span>
      </nav>
      <h1 className="mb-1 text-2xl font-semibold text-ink-900">Labels</h1>
      <p className="mb-6 text-sm text-ink-600">Every label used on pages in this space.</p>

      {isPending && (
        <div className="flex items-center gap-2 py-8 text-sm text-ink-500">
          <Spinner size={14} /> Loading labels
        </div>
      )}

      {error && (
        <div className="rounded-md border border-danger-100 bg-danger-50 px-3 py-3 text-sm text-danger-700">
          <p>{error.message}</p>
          <button
            type="button"
            onClick={() => void refetch()}
            className="mt-2 text-xs font-medium underline"
          >
            Retry
          </button>
        </div>
      )}

      {data && data.length === 0 && (
        <EmptyState
          icon={<Tag size={18} />}
          title="No labels yet"
          description="Add labels to a page from its page view and they will be listed here."
        />
      )}

      {data && data.length > 0 && (
        <ul className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
          {data.map((label) => (
            <li key={label.id}>
              <Link
                href={`/spaces/${key}/labels/${encodeURIComponent(label.name)}`}
                className="surface card-hover flex items-center justify-between gap-3 px-3 py-2.5 text-sm"
              >
                <span className="flex min-w-0 items-center gap-2">
                  <Tag size={14} className="shrink-0 text-ink-500" />
                  <span className="truncate font-medium text-ink-900">{label.name}</span>
                </span>
                <span className="shrink-0 rounded-full bg-ink-100 px-2 py-0.5 text-xs text-ink-600">
                  {label.page_count ?? 0} {label.page_count === 1 ? "page" : "pages"}
                </span>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
