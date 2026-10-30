"use client";

import { useEffect, useMemo, useRef, useState, type RefObject } from "react";
import { applyActiveMark, syncAnchors, type AnchorThread } from "./anchors";

export interface UseInlineAnchorsOptions {
  contentRef?: RefObject<HTMLElement>;
  threads: AnchorThread[];
  activeId: string | null;
  contentKey: string;
  onMarkClick: (id: string) => void;
}

function sameSet(a: ReadonlySet<string> | null, b: ReadonlySet<string>): boolean {
  if (!a || a.size !== b.size) return false;
  for (const v of b) if (!a.has(v)) return false;
  return true;
}

export function useInlineAnchors({
  contentRef,
  threads,
  activeId,
  contentKey,
  onMarkClick,
}: UseInlineAnchorsOptions): ReadonlySet<string> | null {
  const [anchored, setAnchored] = useState<ReadonlySet<string> | null>(null);
  const threadsRef = useRef(threads);
  const activeRef = useRef(activeId);
  const clickRef = useRef(onMarkClick);

  useEffect(() => {
    threadsRef.current = threads;
    activeRef.current = activeId;
    clickRef.current = onMarkClick;
  });

  const signature = useMemo(
    () => threads.map((t) => `${t.id}\u0000${t.quote}\u0000${t.prefix ?? ""}\u0000${t.suffix ?? ""}`).join("\u0001"),
    [threads]
  );

  useEffect(() => {
    let root: HTMLElement | null = null;
    let observer: MutationObserver | null = null;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let attempts = 0;
    let cancelled = false;

    const observe = () => {
      if (root && observer) observer.observe(root, { childList: true, subtree: true });
    };

    const run = () => {
      if (!root || cancelled) return;
      observer?.disconnect();
      const found = syncAnchors(root, threadsRef.current);
      applyActiveMark(root, activeRef.current);
      setAnchored((prev) => (sameSet(prev, found) ? prev : found));
      observe();
    };

    const schedule = () => {
      if (timer) clearTimeout(timer);
      timer = setTimeout(run, 60);
    };

    const onClick = (event: MouseEvent) => {
      const target = event.target as HTMLElement | null;
      const mark = target?.closest<HTMLElement>('span[data-type="comment"]');
      const id = mark?.getAttribute("data-id");
      if (id) clickRef.current(id);
    };

    const attach = () => {
      if (cancelled) return;
      root = contentRef?.current ?? null;
      if (!root) {
        attempts += 1;
        if (attempts < 40) timer = setTimeout(attach, 250);
        return;
      }
      observer = new MutationObserver(schedule);
      root.addEventListener("click", onClick);
      run();
    };

    attach();

    return () => {
      cancelled = true;
      if (timer) clearTimeout(timer);
      observer?.disconnect();
      root?.removeEventListener("click", onClick);
    };
  }, [contentRef, signature, contentKey]);

  useEffect(() => {
    const root = contentRef?.current;
    if (root) applyActiveMark(root, activeId);
  }, [contentRef, activeId]);

  return anchored;
}
