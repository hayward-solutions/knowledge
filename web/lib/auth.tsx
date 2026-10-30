"use client";

import { createContext, useContext, useEffect, useRef, useState, type ReactNode } from "react";
import { usePathname, useRouter } from "next/navigation";
import { useQuery, useQueryClient, type UseQueryResult } from "@tanstack/react-query";
import { ApiError, UNAUTHENTICATED_EVENT, getMe, type User } from "@/lib/api";
import { safeNext } from "@/lib/safe-next";
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

export { safeNext };

const AuthGuardContext = createContext(false);

function FullScreenSpinner() {
  return (
    <div className="flex min-h-screen items-center justify-center bg-ink-50">
      <Spinner size={24} label="Loading" />
    </div>
  );
}

export function RequireAuth({ children }: { children: ReactNode }) {
  const guarded = useContext(AuthGuardContext);
  if (guarded) return <>{children}</>;
  return <AuthGuard>{children}</AuthGuard>;
}

function AuthGuard({ children }: { children: ReactNode }) {
  const router = useRouter();
  const pathname = usePathname();
  const qc = useQueryClient();
  const { data, error, isPending, refetch } = useMe();
  const [expired, setExpired] = useState(false);
  const handled = useRef(false);
  const unauthenticated = error instanceof ApiError && error.status === 401;
  const redirecting = expired || unauthenticated;

  useEffect(() => {
    const onUnauthenticated = () => {
      if (handled.current) return;
      handled.current = true;
      setExpired(true);
    };
    window.addEventListener(UNAUTHENTICATED_EVENT, onUnauthenticated);
    return () => window.removeEventListener(UNAUTHENTICATED_EVENT, onUnauthenticated);
  }, []);

  useEffect(() => {
    if (!redirecting) return;
    handled.current = true;
    const here = `${pathname}${window.location.search}`;
    qc.clear();
    router.replace(`/login?next=${encodeURIComponent(safeNext(here))}`);
  }, [redirecting, pathname, router, qc]);

  if (redirecting || isPending) return <FullScreenSpinner />;

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

  return <AuthGuardContext.Provider value>{children}</AuthGuardContext.Provider>;
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
