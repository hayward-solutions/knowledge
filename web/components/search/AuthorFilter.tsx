"use client";

import { useEffect, useId, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { X } from "lucide-react";
import { useDebouncedValue } from "@/components/labels/useDebouncedValue";
import { Avatar } from "@/components/ui/Avatar";
import { getUser, searchUsers } from "@/lib/api/users";
import { cn } from "@/lib/cn";

export function AuthorFilter({
  value,
  onChange,
}: {
  value: string;
  onChange: (userId: string) => void;
}) {
  const listId = useId();
  const rootRef = useRef<HTMLDivElement>(null);
  const [text, setText] = useState("");
  const [open, setOpen] = useState(false);
  const [index, setIndex] = useState(0);
  const debounced = useDebouncedValue(text.trim(), 200);

  const selected = useQuery({
    queryKey: ["user", value],
    queryFn: () => getUser(value),
    enabled: Boolean(value),
    staleTime: 5 * 60_000,
  });

  const results = useQuery({
    queryKey: ["users", "search", debounced],
    queryFn: () => searchUsers(debounced),
    enabled: open && !value && debounced.length > 0,
    staleTime: 30_000,
  });

  const options = results.data ?? [];

  useEffect(() => {
    setIndex(0);
  }, [options.length, debounced]);

  useEffect(() => {
    if (!open) return;
    function onDoc(e: MouseEvent) {
      if (!rootRef.current?.contains(e.target as Node)) setOpen(false);
    }
    document.addEventListener("mousedown", onDoc);
    return () => document.removeEventListener("mousedown", onDoc);
  }, [open]);

  function pick(id: string) {
    setText("");
    setOpen(false);
    onChange(id);
  }

  if (value) {
    const name = selected.data?.display_name ?? (selected.isError ? "Unknown user" : "Loading");
    return (
      <span className="inline-flex h-8 items-center gap-1.5 rounded-md border border-ink-200 bg-ink-0 pl-2 pr-1 text-sm text-ink-800">
        <Avatar name={name} seed={value} size={18} />
        <span className="max-w-[10rem] truncate">{name}</span>
        <button
          type="button"
          aria-label="Clear author filter"
          onClick={() => onChange("")}
          className="flex h-5 w-5 items-center justify-center rounded-full text-ink-500 hover:bg-ink-100 hover:text-ink-900"
        >
          <X size={12} />
        </button>
      </span>
    );
  }

  return (
    <div ref={rootRef} className="relative">
      <input
        role="combobox"
        aria-label="Author"
        aria-autocomplete="list"
        aria-expanded={open && options.length > 0}
        aria-controls={listId}
        value={text}
        placeholder="Author"
        autoComplete="off"
        onChange={(e) => {
          setText(e.target.value);
          setOpen(true);
        }}
        onFocus={() => setOpen(true)}
        onKeyDown={(e) => {
          if (e.key === "ArrowDown" && options.length > 0) {
            e.preventDefault();
            setIndex((i) => (i + 1) % options.length);
          } else if (e.key === "ArrowUp" && options.length > 0) {
            e.preventDefault();
            setIndex((i) => (i - 1 + options.length) % options.length);
          } else if (e.key === "Enter" && open && options[index]) {
            e.preventDefault();
            pick(options[index].id);
          } else if (e.key === "Escape") {
            setOpen(false);
          }
        }}
        className="control w-40 max-w-full"
      />
      {open && options.length > 0 && (
        <ul
          id={listId}
          role="listbox"
          className="absolute left-0 top-full z-40 mt-1 max-h-60 w-64 max-w-[calc(100vw-2rem)] overflow-y-auto rounded-md border border-ink-200 bg-ink-0 py-1 shadow-overlay"
        >
          {options.map((u, i) => (
            <li
              key={u.id}
              role="option"
              aria-selected={i === index}
              onMouseDown={(e) => e.preventDefault()}
              onMouseEnter={() => setIndex(i)}
              onClick={() => pick(u.id)}
              className={cn(
                "flex cursor-pointer items-center gap-2 px-3 py-1.5 text-sm text-ink-800",
                i === index && "bg-ink-100"
              )}
            >
              <Avatar name={u.display_name} seed={u.id} size={20} />
              <span className="min-w-0 truncate">{u.display_name}</span>
              {u.email && <span className="min-w-0 truncate text-xs text-ink-500">{u.email}</span>}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
