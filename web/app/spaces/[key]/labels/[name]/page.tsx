"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { FileText, Tag } from "lucide-react";
import { listLabelPages } from "@/lib/api/labels";
import { Badge } from "@/components/ui/Badge";
import { EmptyState } from "@/components/ui/EmptyState";
import { Spinner } from "@/components/ui/Spinner";

function decode(raw: string): string {
  try {
    return decodeURIComponent(raw);
  } catch {
    return raw;
  }
}

export default function LabelPagesPage() {
  const params = useParams<{ key: string; name: string }>();
  const key = params.key;
  const name = decode(params.name);

  const { data, isPending, error, refetch } = useQuery({
    queryKey: ["label-pages", key, name],
    queryFn: () => listLabelPages(key, name),
  });

  return (
    <div className="mx-auto w-full max-w-4xl px-4 py-6 sm:px-8">
      <nav aria-label="Breadcrumb" className="mb-3 text-sm text-ink-600">
        <Link href={`/spaces/${key}`} className="hover:text-ink-900 hover:underline">
          {key}
        </Link>
        <span className="mx-1.5 text-ink-400">/</span>
        <Link href={`/spaces/${key}/labels`} className="hover:text-ink-900 hover:underline">
          Labels
        </Link>
        <span className="mx-1.5 text-ink-400">/</span>
        <span className="text-ink-900">{name}</span>
      </nav>
      <h1 className="mb-1 flex items-center gap-2 text-2xl font-semibold text-ink-900">
        <Tag size={20} className="text-ink-500" />
        <span className="min-w-0 break-words">{name}</span>
      </h1>
      <p className="mb-6 text-sm text-ink-600">
        Pages in {key} with this label
        {data ? ` (${data.length})` : ""}.
      </p>

      {isPending && (
        <div className="flex items-center gap-2 py-8 text-sm text-ink-500">
          <Spinner size={14} /> Loading pages
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
          title="No pages with this label"
          description="No page in this space currently carries this label."
          action={
            <Link href={`/spaces/${key}/labels`} className="btn btn-secondary">
              All labels
            </Link>
          }
        />
      )}

      {data && data.length > 0 && (
        <ul className="surface divide-y divide-ink-100 overflow-hidden">
          {data.map((p) => (
            <li key={p.id}>
              <Link
                href={`/spaces/${key}/pages/${p.id}/${p.slug}`}
                className="flex items-center gap-2.5 px-3 py-2.5 text-sm hover:bg-ink-50"
              >
                <FileText size={15} className="shrink-0 text-ink-500" />
                <span className="min-w-0 flex-1 truncate font-medium text-ink-900">{p.title}</span>
                {p.status === "draft" && <Badge tone="amber">Draft</Badge>}
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
