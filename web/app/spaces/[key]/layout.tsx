"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { Archive } from "lucide-react";
import { AppShell } from "@/components/layout/AppShell";
import { RequireAuth } from "@/lib/auth";
import { ApiError } from "@/lib/api";
import { Button } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";
import { ListSkeleton, PageSkeleton } from "@/components/pages/Skeletons";
import { SpaceProvider, useSpaceData } from "@/components/spaces/SpaceContext";
import { SpaceSidebar } from "@/components/spaces/SpaceSidebar";

function SpaceFrame({ spaceKey, children }: { spaceKey: string; children: React.ReactNode }) {
  const { spaceQuery, value } = useSpaceData(spaceKey);

  let sidebar: React.ReactNode;
  let body: React.ReactNode;

  if (value) {
    sidebar = <SpaceSidebar />;
    body = (
      <>
        {value.archived && (
          <div
            role="status"
            className="flex items-center gap-2 border-b border-warning-100 bg-warning-50 px-4 py-2 text-sm text-warning-700 print:hidden sm:px-6"
          >
            <Archive size={14} className="shrink-0" />
            <span>
              This space is archived and read-only.
              {value.isAdmin && (
                <>
                  {" "}
                  <Link href={`/spaces/${value.space.key}/settings/general`} className="font-medium underline">
                    Unarchive it in settings
                  </Link>
                  .
                </>
              )}
            </span>
          </div>
        )}
        {children}
      </>
    );
  } else if (spaceQuery.isPending) {
    sidebar = <ListSkeleton rows={8} />;
    body = <PageSkeleton />;
  } else {
    const status = spaceQuery.error instanceof ApiError ? spaceQuery.error.status : 0;
    sidebar = null;
    body = (
      <div className="mx-auto w-full max-w-xl px-4 py-16">
        <EmptyState
          title={status === 404 ? "Space not found" : status === 403 ? "No access to this space" : "Could not load this space"}
          description={
            status === 404
              ? "This space does not exist, or you do not have access to it."
              : spaceQuery.error?.message
          }
          action={
            status === 404 || status === 403 ? (
              <Link href="/spaces" className="btn btn-primary">
                Browse spaces
              </Link>
            ) : (
              <Button variant="primary" onClick={() => void spaceQuery.refetch()}>
                Try again
              </Button>
            )
          }
        />
      </div>
    );
  }

  return (
    <SpaceProvider value={value}>
      <AppShell sidebar={sidebar}>{body}</AppShell>
    </SpaceProvider>
  );
}

export default function SpaceLayout({ children }: { children: React.ReactNode }) {
  const params = useParams<{ key: string }>();
  const key = decodeURIComponent(String(params.key));
  return (
    <RequireAuth>
      <SpaceFrame spaceKey={key}>{children}</SpaceFrame>
    </RequireAuth>
  );
}
