"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useId, useMemo, useRef, useState, type KeyboardEvent } from "react";
import { useQuery } from "@tanstack/react-query";
import { CornerDownLeft, FileText, Search } from "lucide-react";
import { useDebouncedValue } from "@/components/labels/useDebouncedValue";
import { Badge } from "@/components/ui/Badge";
import { Spinner } from "@/components/ui/Spinner";
import { quickSearch } from "@/lib/api/search";
import { cn } from "@/lib/cn";

interface Item {
  key: string;
  href: string;
  title: string;
  spaceKey?: string;
  spaceName?: string;
  draft?: boolean;
  all?: boolean;
}

function isTypingTarget(target: EventTarget | null): boolean {
  const el = target as HTMLElement | null;
  if (!el || !el.closest) return false;
  return Boolean(el.closest('input, textarea, select, [contenteditable=""], [contenteditable="true"]'));
}

export function QuickSearch() {
  const router = useRouter();
  const listId = useId();
  const rootRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const [q, setQ] = useState("");
  const [open, setOpen] = useState(false);
  const [focused, setFocused] = useState(false);
  const [index, setIndex] = useState(-1);

  const term = q.trim();
  const debounced = useDebouncedValue(term, 150);

  const { data, isFetching } = useQuery({
    queryKey: ["quick-search", debounced],
    queryFn: () => quickSearch(debounced),
    enabled: open,
    staleTime: 15_000,
    placeholderData: (prev) => prev,
  });

  const items = useMemo<Item[]>(() => {
    const list: Item[] = (data ?? []).map((h) => ({
      key: h.page.id,
      href: `/spaces/${h.space_key}/pages/${h.page.id}/${h.page.slug}`,
      title: h.page.title || "Untitled",
      spaceKey: h.space_key,
      spaceName: h.space_name,
      draft: h.page.status === "draft",
    }));
    if (term) {
      list.push({
        key: "__all__",
        href: `/search?q=${encodeURIComponent(term)}`,
        title: term,
        all: true,
      });
    }
    return list;
  }, [data, term]);

  useEffect(() => {
    setIndex(-1);
  }, [debounced]);

  useEffect(() => {
    function onKey(e: globalThis.KeyboardEvent) {
      const input = inputRef.current;
      if (!input) return;
      const cmdK = (e.metaKey || e.ctrlKey) && !e.altKey && !e.shiftKey && e.key.toLowerCase() === "k";
      const slash =
        e.key === "/" && !e.metaKey && !e.ctrlKey && !e.altKey && !isTypingTarget(e.target);
      if (!cmdK && !slash) return;
      e.preventDefault();
      input.focus();
      input.select();
      setOpen(true);
    }
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, []);

  useEffect(() => {
    if (index < 0) return;
    document.getElementById(`${listId}-${index}`)?.scrollIntoView({ block: "nearest" });
  }, [index, listId]);

  function close(blur: boolean) {
    setOpen(false);
    setIndex(-1);
    if (blur) inputRef.current?.blur();
  }

  function go(href: string, clear: boolean) {
    close(true);
    if (clear) setQ("");
    router.push(href);
  }

  function onKeyDown(e: KeyboardEvent<HTMLInputElement>) {
    if (e.nativeEvent.isComposing) return;
    if (e.key === "ArrowDown") {
      e.preventDefault();
      if (!open) {
        setOpen(true);
        return;
      }
      if (items.length > 0) setIndex((i) => (i + 1) % items.length);
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      if (items.length > 0) setIndex((i) => (i <= 0 ? items.length - 1 : i - 1));
    } else if (e.key === "Enter") {
      e.preventDefault();
      const picked = open && index >= 0 ? items[index] : undefined;
      if (picked) go(picked.href, !picked.all);
      else if (term) go(`/search?q=${encodeURIComponent(term)}`, false);
    } else if (e.key === "Escape") {
      e.preventDefault();
      if (open) close(false);
      else inputRef.current?.blur();
    }
  }

  const showPanel = open && (items.length > 0 || isFetching);
  const activeId = showPanel && index >= 0 ? `${listId}-${index}` : undefined;
  const hasHits = items.some((i) => !i.all);

  return (
    <div
      ref={rootRef}
      role="search"
      className="relative w-full"
      onBlur={(e) => {
        if (!rootRef.current?.contains(e.relatedTarget as Node | null)) {
          setOpen(false);
          setIndex(-1);
          setFocused(false);
        }
      }}
    >
      <Search
        size={14}
        className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-ink-400"
      />
      <input
        ref={inputRef}
        type="text"
        role="combobox"
        value={q}
        onChange={(e) => {
          setQ(e.target.value);
          setOpen(true);
        }}
        onFocus={() => {
          setFocused(true);
          setOpen(true);
        }}
        onKeyDown={onKeyDown}
        placeholder="Search"
        aria-label="Search"
        aria-autocomplete="list"
        aria-expanded={showPanel}
        aria-controls={listId}
        aria-activedescendant={activeId}
        aria-keyshortcuts="/ Control+K Meta+K"
        autoComplete="off"
        spellCheck={false}
        className="control w-full pl-8 pr-9"
      />
      <div className="pointer-events-none absolute right-2 top-1/2 -translate-y-1/2">
        {open && isFetching ? (
          <Spinner size={12} />
        ) : (
          !focused &&
          !q && (
            <kbd className="hidden rounded-xs border border-ink-200 bg-ink-50 px-1.5 py-0.5 font-mono text-[10px] text-ink-500 md:inline">
              /
            </kbd>
          )
        )}
      </div>

      {showPanel && (
        <div
          onMouseDown={(e) => e.preventDefault()}
          className="fixed inset-x-2 top-[3.5rem] z-50 overflow-hidden rounded-md border border-ink-200 bg-ink-0 shadow-overlay sm:absolute sm:inset-x-auto sm:left-0 sm:top-full sm:mt-1 sm:w-[28rem] sm:max-w-[calc(100vw-2rem)]"
        >
          <ul id={listId} role="listbox" aria-label="Quick search results" className="max-h-[60vh] overflow-y-auto py-1">
            {!term && hasHits && (
              <li role="presentation" className="px-3 pb-1 pt-1.5 text-[11px] font-semibold uppercase tracking-wide text-ink-500">
                Recently updated
              </li>
            )}
            {term && !hasHits && !isFetching && (
              <li role="presentation" className="px-3 py-2 text-sm text-ink-500">
                No pages match that title
              </li>
            )}
            {items.map((item, i) => (
              <li
                key={item.key}
                id={`${listId}-${i}`}
                role="option"
                aria-selected={i === index}
                onMouseEnter={() => setIndex(i)}
              >
                <Link
                  href={item.href}
                  tabIndex={-1}
                  onClick={() => {
                    close(true);
                    if (!item.all) setQ("");
                  }}
                  className={cn(
                    "flex items-center gap-2 px-3 py-2 text-sm text-ink-800",
                    i === index && "bg-ink-100"
                  )}
                >
                  {item.all ? (
                    <>
                      <Search size={14} className="shrink-0 text-ink-500" />
                      <span className="min-w-0 flex-1 truncate">
                        Search for <span className="font-semibold">&ldquo;{item.title}&rdquo;</span>
                      </span>
                      <CornerDownLeft size={13} className="shrink-0 text-ink-400" />
                    </>
                  ) : (
                    <>
                      <FileText size={14} className="shrink-0 text-ink-500" />
                      <span className="min-w-0 flex-1 truncate">{item.title}</span>
                      {item.draft && <Badge tone="amber">Draft</Badge>}
                      {item.spaceKey && (
                        <Badge tone="blue" title={item.spaceName}>
                          {item.spaceKey}
                        </Badge>
                      )}
                    </>
                  )}
                </Link>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}

export default QuickSearch;
