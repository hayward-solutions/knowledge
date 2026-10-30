"use client";

import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { listSpaces } from "@/lib/api";

export function useSpaceKeys(enabled = true): Map<string, string> {
  const { data } = useQuery({
    queryKey: ["spaces"],
    queryFn: listSpaces,
    enabled,
    staleTime: 60_000,
  });
  return useMemo(() => {
    const m = new Map<string, string>();
    for (const s of data ?? []) m.set(s.id, s.key);
    return m;
  }, [data]);
}
