"use client";

import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { LayoutGrid, Plus, Search } from "lucide-react";
import { AppShell } from "@/components/layout/AppShell";
import { listSpaces, type Space } from "@/lib/api";
import { Button } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";
import { Input } from "@/components/ui/Input";
import { Skeleton } from "@/components/pages/Skeletons";
import { CreateSpaceDialog } from "@/components/spaces/CreateSpaceDialog";
import { SpaceCard } from "@/components/spaces/SpaceCard";

export default function SpacesDirectoryPage() {
  const [filter, setFilter] = useState("");
  const [creating, setCreating] = useState(false);
  const query = useQuery<Space[], Error>({ queryKey: ["spaces"], queryFn: listSpaces });

  const spaces = useMemo(() => {
    const q = filter.trim().toLowerCase();
    return [...(query.data ?? [])]
      .filter(
        (s) =>
          !q ||
          s.name.toLowerCase().includes(q) ||
          s.key.toLowerCase().includes(q) ||
          (s.description ?? "").toLowerCase().includes(q)
      )
      .sort((a, b) => {
        const aa = a.archived_at ? 1 : 0;
        const ba = b.archived_at ? 1 : 0;
        return aa - ba || a.name.localeCompare(b.name);
      });
  }, [query.data, filter]);

  return (
    <AppShell>
      <div className="border-b border-ink-200 bg-ink-0 px-4 py-5 sm:px-6">
        <div className="mx-auto flex w-full max-w-6xl flex-wrap items-center justify-between gap-3">
          <div>
            <h1 className="text-[22px] font-semibold tracking-tight text-ink-900">Spaces</h1>
            <p className="text-sm text-ink-600">Browse the spaces you can access, or start a new one.</p>
          </div>
          <Button variant="primary" onClick={() => setCreating(true)}>
            <Plus size={14} />
            Create space
          </Button>
        </div>
      </div>

      <div className="flex-1 px-4 py-6 sm:px-6">
        <div className="mx-auto w-full max-w-6xl">
          <div className="relative mb-5 max-w-md">
            <Search size={14} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-ink-400" />
            <Input
              type="search"
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
              placeholder="Filter spaces"
              aria-label="Filter spaces"
              className="pl-8"
            />
          </div>

          {query.isPending ? (
            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3" aria-busy="true">
              {Array.from({ length: 6 }).map((_, i) => (
                <Skeleton key={i} className="h-36 w-full" />
              ))}
            </div>
          ) : query.isError ? (
            <EmptyState
              title="Could not load spaces"
              description={query.error.message}
              action={
                <Button variant="primary" onClick={() => void query.refetch()}>
                  Try again
                </Button>
              }
            />
          ) : spaces.length === 0 ? (
            <EmptyState
              icon={<LayoutGrid size={18} />}
              title={filter ? "No spaces match your filter" : "No spaces yet"}
              description={
                filter ? "Try a different name or key." : "Create the first space to start writing pages."
              }
              action={
                filter ? (
                  <Button onClick={() => setFilter("")}>Clear filter</Button>
                ) : (
                  <Button variant="primary" onClick={() => setCreating(true)}>
                    Create space
                  </Button>
                )
              }
            />
          ) : (
            <ul className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
              {spaces.map((s) => (
                <li key={s.id}>
                  <SpaceCard space={s} />
                </li>
              ))}
            </ul>
          )}
        </div>
      </div>

      {creating && <CreateSpaceDialog onClose={() => setCreating(false)} />}
    </AppShell>
  );
}
