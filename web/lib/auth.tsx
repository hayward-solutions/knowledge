"use client";

import { useEffect, type ReactNode } from "react";
import { usePathname, useRouter } from "next/navigation";
import { useQuery, type UseQueryResult } from "@tanstack/react-query";
import { ApiError, getMe, type User } from "@/lib/api";
import { Button } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";
import { Spinner } from "@/components/ui/Spinner";

export const ME_QUERY_KEY = ["me"] as const;

export function useMe(): UseQueryResult<User, Error> {
  return useQuery<User, Error>({
    queryKey: ME_QUERY_KEY,
    queryFn: getMe,
    retry: false,
    staleTime: 60_000,
  });
}

export function safeNext(raw: string | null | undefined, fallback = "/"): string {
  if (!raw) return fallback;
  if (!raw.startsWith("/")) return fallback;
  if (raw.startsWith("//") || raw.startsWith("/\\")) return fallback;
  return raw;
}

function FullScreenSpinner() {
  return (
    <div className="flex min-h-screen items-center justify-center bg-ink-50">
      <Spinner size={24} label="Loading" />
    </div>
  );
}

export function RequireAuth({ children }: { children: ReactNode }) {
  const router = useRouter();
  const pathname = usePathname();
  const { data, error, isPending, refetch } = useMe();
  const unauthenticated = error instanceof ApiError && error.status === 401;

  useEffect(() => {
    if (!unauthenticated) return;
    const here = `${pathname}${window.location.search}`;
    router.replace(`/login?next=${encodeURIComponent(safeNext(here))}`);
  }, [unauthenticated, pathname, router]);

  if (isPending || unauthenticated) return <FullScreenSpinner />;

  if (error || !data) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-ink-50 p-6">
        <EmptyState
          title="Could not reach the server"
          description={error?.message || "Something went wrong while loading your session."}
          action={
            <Button variant="primary" onClick={() => refetch()}>
              Try again
            </Button>
          }
        />
      </div>
    );
  }

  return <>{children}</>;
}

export function RequireAdmin({ children }: { children: ReactNode }) {
  return (
    <RequireAuth>
      <AdminGate>{children}</AdminGate>
    </RequireAuth>
  );
}

function AdminGate({ children }: { children: ReactNode }) {
  const { data } = useMe();
  if (!data?.is_system_admin) {
    return (
      <div className="p-6">
        <EmptyState
          title="Administrators only"
          description="You need system administrator access to view this page."
        />
      </div>
    );
  }
  return <>{children}</>;
}
