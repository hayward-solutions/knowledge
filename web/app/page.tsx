"use client";

import { useState } from "react";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { Clock, Eye, FilePen, LayoutGrid, Plus, Star } from "lucide-react";
import { AppShell } from "@/components/layout/AppShell";
import { getDashboard, type Dashboard, type DashboardPage } from "@/lib/api";
import { useMe } from "@/lib/auth";
import { Button } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";
import { PageRow } from "@/components/pages/PageRow";
import { ListSkeleton } from "@/components/pages/Skeletons";
import { CreateSpaceDialog } from "@/components/spaces/CreateSpaceDialog";

function Section({
  title,
  icon,
  pages,
  loading,
  emptyTitle,
  emptyDescription,
  edit,
}: {
  title: string;
  icon: React.ReactNode;
  pages: DashboardPage[] | undefined;
  loading: boolean;
  emptyTitle: string;
  emptyDescription: string;
  edit?: boolean;
}) {
  return (
    <section aria-label={title} className="min-w-0">
      <h2 className="mb-2 flex items-center gap-2 text-sm font-semibold text-ink-900">
        <span className="text-ink-500">{icon}</span>
        {title}
      </h2>
      {loading ? (
        <ListSkeleton rows={3} />
      ) : !pages || pages.length === 0 ? (
        <div className="rounded-lg border border-dashed border-ink-200 bg-ink-0 px-4 py-6 text-center">
          <div className="text-sm font-medium text-ink-800">{emptyTitle}</div>
          <p className="mt-0.5 text-xs text-ink-600">{emptyDescription}</p>
        </div>
      ) : (
        <ul className="surface divide-y divide-ink-100 overflow-hidden">
          {pages.map((p) => (
            <PageRow key={p.id} page={p} edit={edit} />
          ))}
        </ul>
      )}
    </section>
  );
}

export default function HomePage() {
  const { data: me } = useMe();
  const [creating, setCreating] = useState(false);
  const query = useQuery<Dashboard, Error>({ queryKey: ["dashboard"], queryFn: getDashboard, staleTime: 0 });
  const data = query.data;

  return (
    <AppShell>
      <div className="border-b border-ink-200 bg-ink-0 px-4 py-5 sm:px-6">
        <div className="mx-auto flex w-full max-w-6xl flex-wrap items-center justify-between gap-3">
          <div>
            <h1 className="text-[22px] font-semibold tracking-tight text-ink-900">
              Welcome{me ? `, ${me.display_name || me.email}` : ""}
            </h1>
            <p className="text-sm text-ink-600">Pick up where you left off.</p>
          </div>
          <div className="flex items-center gap-2">
            <Link href="/spaces" className="btn btn-secondary">
              <LayoutGrid size={14} />
              Browse spaces
            </Link>
            <Button variant="primary" onClick={() => setCreating(true)}>
              <Plus size={14} />
              Create space
            </Button>
          </div>
        </div>
      </div>

      <div className="flex-1 px-4 py-6 sm:px-6">
        <div className="mx-auto w-full max-w-6xl">
          {query.isError ? (
            <EmptyState
              title="Could not load your dashboard"
              description={query.error.message}
              action={
                <Button variant="primary" onClick={() => void query.refetch()}>
                  Try again
                </Button>
              }
            />
          ) : (
            <div className="grid gap-6 lg:grid-cols-2">
              <Section
                title="Recently updated"
                icon={<Clock size={16} />}
                pages={data?.recent_updates}
                loading={query.isPending}
                emptyTitle="Nothing updated yet"
                emptyDescription="Pages updated in your spaces will appear here."
              />
              <Section
                title="Recently viewed"
                icon={<Eye size={16} />}
                pages={data?.recently_viewed}
                loading={query.isPending}
                emptyTitle="No pages viewed yet"
                emptyDescription="Pages you open will appear here."
              />
              <Section
                title="My drafts"
                icon={<FilePen size={16} />}
                pages={data?.my_drafts}
                loading={query.isPending}
                emptyTitle="No drafts"
                emptyDescription="Unpublished pages and changes you are working on show up here."
                edit
              />
              <Section
                title="Starred"
                icon={<Star size={16} />}
                pages={data?.starred}
                loading={query.isPending}
                emptyTitle="No starred pages"
                emptyDescription="Star a page to keep it within reach."
              />
            </div>
          )}
        </div>
      </div>

      {creating && <CreateSpaceDialog onClose={() => setCreating(false)} />}
    </AppShell>
  );
}
