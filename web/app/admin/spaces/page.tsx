"use client";

import { useMemo, useState } from "react";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { Search } from "lucide-react";
import { listAdminSpaces } from "@/lib/api";
import { Badge } from "@/components/ui/Badge";
import { EmptyState } from "@/components/ui/EmptyState";
import { Input } from "@/components/ui/Input";
import { Spinner } from "@/components/ui/Spinner";

export default function AdminSpacesPage() {
  const spaces = useQuery({ queryKey: ["admin", "spaces"], queryFn: listAdminSpaces });
  const [search, setSearch] = useState("");

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    const list = spaces.data ?? [];
    if (!q) return list;
    return list.filter(
      (s) => s.key.toLowerCase().includes(q) || s.name.toLowerCase().includes(q)
    );
  }, [spaces.data, search]);

  return (
    <>
      <div className="border-b border-ink-200 bg-ink-0 px-4 py-4 sm:px-6">
        <h2 className="text-base font-semibold text-ink-900">Spaces</h2>
        <p className="text-sm text-ink-600">
          Every space in this instance, including archived and deleted ones.
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
            placeholder="Search by key or name…"
            aria-label="Search spaces"
            className="pl-8"
          />
        </div>
      </div>

      <div className="flex-1 overflow-auto bg-ink-0">
        {spaces.isPending ? (
          <div className="flex justify-center py-16">
            <Spinner size={20} />
          </div>
        ) : spaces.isError ? (
          <p role="alert" className="p-6 text-sm text-danger-600">
            {spaces.error.message}
          </p>
        ) : filtered.length === 0 ? (
          <div className="p-6">
            <EmptyState title={search ? "No spaces match your search" : "No spaces"} />
          </div>
        ) : (
          <table className="w-full min-w-[760px] border-separate border-spacing-0 text-sm">
            <thead className="sticky top-0 z-10 bg-ink-50 text-left text-[10.5px] font-semibold uppercase tracking-wider text-ink-600">
              <tr>
                <th className="border-b border-ink-200 px-4 py-2">Key</th>
                <th className="border-b border-ink-200 px-3 py-2">Name</th>
                <th className="border-b border-ink-200 px-3 py-2">Visibility</th>
                <th className="border-b border-ink-200 px-3 py-2 text-right">Members</th>
                <th className="border-b border-ink-200 px-3 py-2 text-right">Pages</th>
                <th className="border-b border-ink-200 px-3 py-2">Status</th>
                <th className="border-b border-ink-200 px-4 py-2">Created</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((s) => {
                const deleted = Boolean(s.deleted_at);
                return (
                  <tr key={s.id} className="hover:bg-brand-50/40">
                    <td className="whitespace-nowrap border-b border-ink-100 px-4 py-2 font-mono text-xs font-semibold text-ink-800">
                      {deleted ? (
                        s.key
                      ) : (
                        <Link href={`/spaces/${s.key}`} className="link">
                          {s.key}
                        </Link>
                      )}
                    </td>
                    <td className="border-b border-ink-100 px-3 py-2">
                      <div className="font-medium text-ink-900">{s.name}</div>
                      {s.description && (
                        <div className="max-w-md truncate text-xs text-ink-500">{s.description}</div>
                      )}
                    </td>
                    <td className="border-b border-ink-100 px-3 py-2">
                      <Badge tone={s.visibility === "private" ? "amber" : "blue"}>
                        {s.visibility === "private" ? "Private" : "Internal"}
                      </Badge>
                    </td>
                    <td className="border-b border-ink-100 px-3 py-2 text-right tabular-nums text-ink-700">
                      {s.member_count}
                    </td>
                    <td className="border-b border-ink-100 px-3 py-2 text-right tabular-nums text-ink-700">
                      {s.page_count}
                    </td>
                    <td className="border-b border-ink-100 px-3 py-2">
                      <div className="flex flex-wrap gap-1">
                        {deleted ? (
                          <Badge tone="red">Deleted</Badge>
                        ) : s.archived_at ? (
                          <Badge tone="neutral">Archived</Badge>
                        ) : (
                          <Badge tone="green">Active</Badge>
                        )}
                      </div>
                    </td>
                    <td className="whitespace-nowrap border-b border-ink-100 px-4 py-2 text-ink-600">
                      {new Date(s.created_at).toLocaleDateString()}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </div>
    </>
  );
}
