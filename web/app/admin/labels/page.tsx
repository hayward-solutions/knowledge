"use client";

import { useMemo, useState } from "react";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { Search } from "lucide-react";
import { listLabels } from "@/lib/api";
import { EmptyState } from "@/components/ui/EmptyState";
import { Input } from "@/components/ui/Input";
import { Spinner } from "@/components/ui/Spinner";

export default function AdminLabelsPage() {
  const labels = useQuery({ queryKey: ["admin", "labels"], queryFn: () => listLabels() });
  const [search, setSearch] = useState("");

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    const list = (labels.data ?? []).slice().sort((a, b) => {
      const diff = (b.page_count ?? 0) - (a.page_count ?? 0);
      return diff !== 0 ? diff : a.name.localeCompare(b.name);
    });
    return q ? list.filter((l) => l.name.toLowerCase().includes(q)) : list;
  }, [labels.data, search]);

  return (
    <>
      <div className="border-b border-ink-200 bg-ink-0 px-4 py-4 sm:px-6">
        <h2 className="text-base font-semibold text-ink-900">Labels</h2>
        <p className="text-sm text-ink-600">
          Labels in use across all spaces. This view is read-only; renaming and merging labels is
          not supported yet.
        </p>
      </div>

      <div className="border-b border-ink-200 bg-ink-0 px-4 py-3 sm:px-6">
        <div className="relative max-w-sm">
          <Search
            size={14}
            className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-ink-400"
          />
          <Input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Filter labels…"
            aria-label="Filter labels"
            className="pl-8"
          />
        </div>
      </div>

      <div className="flex-1 overflow-auto bg-ink-0">
        {labels.isPending ? (
          <div className="flex justify-center py-16">
            <Spinner size={20} />
          </div>
        ) : labels.isError ? (
          <p role="alert" className="p-6 text-sm text-danger-600">
            {labels.error.message}
          </p>
        ) : filtered.length === 0 ? (
          <div className="p-6">
            <EmptyState title={search ? "No labels match your filter" : "No labels yet"} />
          </div>
        ) : (
          <table className="w-full min-w-[420px] border-separate border-spacing-0 text-sm">
            <thead className="sticky top-0 z-10 bg-ink-50 text-left text-[10.5px] font-semibold uppercase tracking-wider text-ink-600">
              <tr>
                <th className="border-b border-ink-200 px-4 py-2">Label</th>
                <th className="border-b border-ink-200 px-3 py-2 text-right">Pages</th>
                <th className="border-b border-ink-200 px-4 py-2">Created</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((l) => (
                <tr key={l.id} className="hover:bg-brand-50/40">
                  <td className="border-b border-ink-100 px-4 py-2">
                    <Link
                      href={`/search?label=${encodeURIComponent(l.name)}`}
                      className="chip bg-ink-100 normal-case text-ink-800 hover:bg-ink-200"
                    >
                      {l.name}
                    </Link>
                  </td>
                  <td className="border-b border-ink-100 px-3 py-2 text-right tabular-nums text-ink-700">
                    {l.page_count ?? 0}
                  </td>
                  <td className="whitespace-nowrap border-b border-ink-100 px-4 py-2 text-ink-600">
                    {l.created_at ? new Date(l.created_at).toLocaleDateString() : ""}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </>
  );
}
