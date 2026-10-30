"use client";

import Link from "next/link";
import { cn } from "@/lib/cn";

export interface TabItem<T extends string = string> {
  id: T;
  label: string;
  href?: string;
  count?: number;
}

export function Tabs<T extends string = string>({
  tabs,
  value,
  onChange,
  className,
}: {
  tabs: TabItem<T>[];
  value: T;
  onChange?: (id: T) => void;
  className?: string;
}) {
  return (
    <div
      role="tablist"
      className={cn("flex items-center gap-1 overflow-x-auto border-b border-ink-200", className)}
    >
      {tabs.map((t) => {
        const active = t.id === value;
        const cls = cn(
          "relative -mb-px flex h-9 shrink-0 items-center gap-1.5 whitespace-nowrap border-b-2 px-3 text-sm font-medium transition",
          active
            ? "border-brand-600 text-brand-700"
            : "border-transparent text-ink-600 hover:text-ink-900"
        );
        const content = (
          <>
            {t.label}
            {t.count !== undefined && (
              <span className="rounded-full bg-ink-100 px-1.5 text-[11px] text-ink-600">
                {t.count}
              </span>
            )}
          </>
        );
        if (t.href) {
          return (
            <Link
              key={t.id}
              href={t.href}
              role="tab"
              aria-selected={active}
              className={cls}
            >
              {content}
            </Link>
          );
        }
        return (
          <button
            key={t.id}
            type="button"
            role="tab"
            aria-selected={active}
            onClick={() => onChange?.(t.id)}
            className={cls}
          >
            {content}
          </button>
        );
      })}
    </div>
  );
}
