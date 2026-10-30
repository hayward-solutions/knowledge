"use client";

import { createContext, useCallback, useContext, useMemo, type ReactNode } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { getSpace, getSpaceTree, type PageSummary, type Role, type Space } from "@/lib/api";
import { useMe } from "@/lib/auth";

export interface SpaceContextValue {
  space: Space;
  role: Role;
  tree: PageSummary[];
  treeLoading: boolean;
  treeError: Error | null;
  isAdmin: boolean;
  isEditor: boolean;
  archived: boolean;
  canEdit: boolean;
  refresh: () => Promise<void>;
}

const SpaceContext = createContext<SpaceContextValue | null>(null);

export function SpaceProvider({
  value,
  children,
}: {
  value: SpaceContextValue | null;
  children: ReactNode;
}) {
  return <SpaceContext.Provider value={value}>{children}</SpaceContext.Provider>;
}

export function useSpace(): SpaceContextValue {
  const ctx = useContext(SpaceContext);
  if (!ctx) throw new Error("useSpace must be used inside the space layout");
  return ctx;
}

export function useOptionalSpace(): SpaceContextValue | null {
  return useContext(SpaceContext);
}

export function useSpaceData(key: string) {
  const qc = useQueryClient();
  const { data: me } = useMe();
  const spaceQuery = useQuery<Space, Error>({
    queryKey: ["space", key],
    queryFn: () => getSpace(key),
    retry: false,
  });
  const treeQuery = useQuery<PageSummary[], Error>({
    queryKey: ["tree", key],
    queryFn: () => getSpaceTree(key),
    retry: false,
  });

  const refresh = useCallback(async () => {
    await Promise.all([
      qc.invalidateQueries({ queryKey: ["space", key] }),
      qc.invalidateQueries({ queryKey: ["tree", key] }),
    ]);
  }, [qc, key]);

  const space = spaceQuery.data;
  const value = useMemo<SpaceContextValue | null>(() => {
    if (!space) return null;
    const role: Role = me?.is_system_admin ? "admin" : (space.role ?? "viewer");
    const archived = !!space.archived_at;
    const isAdmin = role === "admin";
    const isEditor = role === "admin" || role === "editor";
    return {
      space,
      role,
      tree: treeQuery.data ?? [],
      treeLoading: treeQuery.isPending,
      treeError: treeQuery.error,
      isAdmin,
      isEditor,
      archived,
      canEdit: isEditor && !archived,
      refresh,
    };
  }, [space, me?.is_system_admin, treeQuery.data, treeQuery.isPending, treeQuery.error, refresh]);

  return { spaceQuery, treeQuery, value };
}
