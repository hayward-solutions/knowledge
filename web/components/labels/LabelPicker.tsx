"use client";

import { useEffect, useId, useMemo, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { listLabels } from "@/lib/api/labels";
import { cn } from "@/lib/cn";
import { labelError, normalizeLabel } from "./normalize";
import { useDebouncedValue } from "./useDebouncedValue";

export interface LabelPickerProps {
  selected?: string[];
  onSelect: (name: string) => void;
  onClose?: () => void;
  placeholder?: string;
  disabled?: boolean;
  autoFocus?: boolean;
  allowCreate?: boolean;
  closeOnSelect?: boolean;
  ariaLabel?: string;
  className?: string;
}

interface Option {
  name: string;
  count?: number;
  create?: boolean;
}

export function LabelPicker({
  selected = [],
  onSelect,
  onClose,
  placeholder = "Add label",
  disabled = false,
  autoFocus = false,
  allowCreate = true,
  closeOnSelect = false,
  ariaLabel = "Label",
  className,
}: LabelPickerProps) {
  const listId = useId();
  const inputRef = useRef<HTMLInputElement>(null);
  const rootRef = useRef<HTMLDivElement>(null);
  const [text, setText] = useState("");
  const [open, setOpen] = useState(false);
  const [index, setIndex] = useState(0);
  const [error, setError] = useState<string | null>(null);

  const normalized = normalizeLabel(text);
  const debounced = useDebouncedValue(normalized, 150);

  const query = useQuery({
    queryKey: ["labels", "autocomplete", debounced],
    queryFn: () => listLabels(debounced),
    enabled: open && !disabled,
    staleTime: 30_000,
    placeholderData: (prev) => prev,
  });

  const options = useMemo<Option[]>(() => {
    const chosen = new Set(selected);
    const list: Option[] = (query.data ?? [])
      .filter((l) => !chosen.has(l.name) && l.name.startsWith(normalized))
      .map((l) => ({ name: l.name, count: l.page_count }));
    if (
      allowCreate &&
      normalized &&
      !chosen.has(normalized) &&
      !list.some((o) => o.name === normalized)
    ) {
      list.push({ name: normalized, create: true });
    }
    return list;
  }, [query.data, selected, normalized, allowCreate]);

  useEffect(() => {
    setIndex(0);
  }, [options.length, debounced]);

  useEffect(() => {
    if (autoFocus) inputRef.current?.focus();
  }, [autoFocus]);

  useEffect(() => {
    if (!open) return;
    function onDoc(e: MouseEvent) {
      if (!rootRef.current?.contains(e.target as Node)) setOpen(false);
    }
    document.addEventListener("mousedown", onDoc);
    return () => document.removeEventListener("mousedown", onDoc);
  }, [open]);

  function commit(name: string) {
    const err = labelError(name);
    if (err) {
      setError(err);
      return;
    }
    if (selected.includes(name)) {
      setError("That label is already added");
      return;
    }
    setError(null);
    setText("");
    onSelect(name);
    if (closeOnSelect) {
      setOpen(false);
      onClose?.();
    } else {
      inputRef.current?.focus();
    }
  }

  function onKeyDown(e: React.KeyboardEvent<HTMLInputElement>) {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      setOpen(true);
      if (options.length > 0) setIndex((i) => (i + 1) % options.length);
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      if (options.length > 0) setIndex((i) => (i - 1 + options.length) % options.length);
    } else if (e.key === "Enter" || e.key === ",") {
      e.preventDefault();
      const picked = open && options[index] ? options[index].name : normalized;
      if (picked) commit(picked);
    } else if (e.key === "Escape") {
      e.preventDefault();
      if (open) setOpen(false);
      else onClose?.();
    } else if (e.key === "Backspace" && text === "") {
      setError(null);
    }
  }

  const activeId = open && options[index] ? `${listId}-${index}` : undefined;

  return (
    <div ref={rootRef} className={cn("relative", className)}>
      <input
        ref={inputRef}
        role="combobox"
        aria-label={ariaLabel}
        aria-autocomplete="list"
        aria-expanded={open}
        aria-controls={listId}
        aria-activedescendant={activeId}
        aria-invalid={error ? true : undefined}
        value={text}
        disabled={disabled}
        placeholder={placeholder}
        maxLength={80}
        autoComplete="off"
        onChange={(e) => {
          setText(e.target.value);
          setError(null);
          setOpen(true);
        }}
        onFocus={() => setOpen(true)}
        onKeyDown={onKeyDown}
        className="control w-44 max-w-full"
      />
      {open && options.length > 0 && (
        <ul
          id={listId}
          role="listbox"
          className="absolute left-0 top-full z-40 mt-1 max-h-60 w-60 max-w-[calc(100vw-2rem)] overflow-y-auto rounded-md border border-ink-200 bg-ink-0 py-1 shadow-overlay"
        >
          {options.map((o, i) => (
            <li
              key={`${o.create ? "new:" : ""}${o.name}`}
              id={`${listId}-${i}`}
              role="option"
              aria-selected={i === index}
              onMouseDown={(e) => e.preventDefault()}
              onMouseEnter={() => setIndex(i)}
              onClick={() => commit(o.name)}
              className={cn(
                "flex cursor-pointer items-center justify-between gap-2 px-3 py-1.5 text-sm text-ink-800",
                i === index && "bg-ink-100"
              )}
            >
              <span className="min-w-0 truncate">
                {o.create ? (
                  <>
                    Create <span className="font-semibold">{o.name}</span>
                  </>
                ) : (
                  o.name
                )}
              </span>
              {o.count !== undefined && (
                <span className="shrink-0 text-xs text-ink-500">{o.count}</span>
              )}
            </li>
          ))}
        </ul>
      )}
      {error && (
        <p role="alert" className="mt-1 max-w-xs text-xs text-danger-600">
          {error}
        </p>
      )}
    </div>
  );
}
