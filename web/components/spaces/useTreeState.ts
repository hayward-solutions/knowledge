"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";

function storageKey(spaceKey: string): string {
  return `knowledge.tree.${spaceKey}`;
}

function readStored(spaceKey: string): string[] | null {
  try {
    const raw = window.localStorage.getItem(storageKey(spaceKey));
    if (!raw) return null;
    const parsed: unknown = JSON.parse(raw);
    if (Array.isArray(parsed) && parsed.every((x) => typeof x === "string")) return parsed as string[];
    return null;
  } catch {
    return null;
  }
}

function writeStored(spaceKey: string, ids: string[]) {
  try {
    window.localStorage.setItem(storageKey(spaceKey), JSON.stringify(ids));
  } catch {
    return;
  }
}

export interface TreeState {
  expanded: Set<string>;
  isExpanded: (id: string) => boolean;
  toggle: (id: string) => void;
  expand: (ids: string[]) => void;
  collapse: (id: string) => void;
}

export function useTreeState(spaceKey: string, defaultExpanded: string[]): TreeState {
  const [stored, setStored] = useState<string[] | null>(null);
  const loaded = useRef(false);
  const defaultKey = defaultExpanded.join("|");

  useEffect(() => {
    loaded.current = false;
    setStored(readStored(spaceKey));
    loaded.current = true;
  }, [spaceKey]);

  const expanded = useMemo(
    () => new Set(stored ?? (defaultKey ? defaultKey.split("|") : [])),
    [stored, defaultKey]
  );

  const commit = useCallback(
    (next: Set<string>) => {
      const list = Array.from(next);
      setStored(list);
      if (loaded.current) writeStored(spaceKey, list);
    },
    [spaceKey]
  );

  const toggle = useCallback(
    (id: string) => {
      const next = new Set(expanded);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      commit(next);
    },
    [expanded, commit]
  );

  const expand = useCallback(
    (ids: string[]) => {
      if (ids.every((id) => expanded.has(id))) return;
      const next = new Set(expanded);
      ids.forEach((id) => next.add(id));
      commit(next);
    },
    [expanded, commit]
  );

  const collapse = useCallback(
    (id: string) => {
      if (!expanded.has(id)) return;
      const next = new Set(expanded);
      next.delete(id);
      commit(next);
    },
    [expanded, commit]
  );

  const isExpanded = useCallback((id: string) => expanded.has(id), [expanded]);

  return { expanded, isExpanded, toggle, expand, collapse };
}
