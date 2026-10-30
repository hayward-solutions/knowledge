"use client";

import {
  forwardRef,
  useEffect,
  useImperativeHandle,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { cn } from "@/lib/cn";

export interface SuggestionListItem<T = unknown> {
  key: string;
  label: string;
  description?: string;
  icon?: ReactNode;
  data: T;
}

export interface SuggestionListProps<T = unknown> {
  items: SuggestionListItem<T>[];
  command: (item: SuggestionListItem<T>) => void;
  emptyLabel?: string;
  loading?: boolean;
}

export interface SuggestionListHandle {
  onKeyDown: (event: KeyboardEvent) => boolean;
}

function SuggestionListInner<T>(
  { items, command, emptyLabel = "No results", loading }: SuggestionListProps<T>,
  ref: React.Ref<SuggestionListHandle>
) {
  const [index, setIndex] = useState(0);
  const listRef = useRef<HTMLDivElement>(null);
  const itemsKey = items.map((i) => i.key).join("|");

  useEffect(() => {
    setIndex(0);
  }, [itemsKey]);

  useEffect(() => {
    const el = listRef.current?.querySelector<HTMLElement>(`[data-index="${index}"]`);
    el?.scrollIntoView({ block: "nearest" });
  }, [index]);

  useImperativeHandle(
    ref,
    () => ({
      onKeyDown: (event) => {
        if (items.length === 0) return false;
        if (event.key === "ArrowUp") {
          setIndex((i) => (i + items.length - 1) % items.length);
          return true;
        }
        if (event.key === "ArrowDown") {
          setIndex((i) => (i + 1) % items.length);
          return true;
        }
        if (event.key === "Enter" || event.key === "Tab") {
          const item = items[index];
          if (item) command(item);
          return true;
        }
        return false;
      },
    }),
    [items, index, command]
  );

  return (
    <div ref={listRef} className="kn-suggest" role="listbox">
      {items.length === 0 ? (
        <div className="kn-suggest-empty">{loading ? "Searching..." : emptyLabel}</div>
      ) : (
        items.map((item, i) => (
          <button
            key={item.key}
            type="button"
            role="option"
            aria-selected={i === index}
            data-index={i}
            className={cn("kn-suggest-item", i === index && "is-active")}
            onMouseDown={(e) => e.preventDefault()}
            onMouseEnter={() => setIndex(i)}
            onClick={() => command(item)}
          >
            {item.icon && <span className="kn-suggest-icon">{item.icon}</span>}
            <span className="kn-suggest-text">
              <span className="kn-suggest-label">{item.label}</span>
              {item.description && <span className="kn-suggest-desc">{item.description}</span>}
            </span>
          </button>
        ))
      )}
    </div>
  );
}

export const SuggestionList = forwardRef(SuggestionListInner) as <T>(
  props: SuggestionListProps<T> & { ref?: React.Ref<SuggestionListHandle> }
) => ReturnType<typeof SuggestionListInner>;
