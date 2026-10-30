"use client";

import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { cn } from "@/lib/cn";

export interface ContextMenuItem {
  key: string;
  label: string;
  icon?: React.ReactNode;
  tone?: "default" | "danger";
  separatorBefore?: boolean;
  onSelect: () => void;
}

export function TreeContextMenu({
  x,
  y,
  items,
  label,
  onClose,
}: {
  x: number;
  y: number;
  items: ContextMenuItem[];
  label: string;
  onClose: () => void;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const [pos, setPos] = useState({ left: x, top: y });

  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const r = el.getBoundingClientRect();
    const left = Math.max(8, Math.min(x, window.innerWidth - r.width - 8));
    const top = Math.max(8, Math.min(y, window.innerHeight - r.height - 8));
    setPos({ left, top });
    el.querySelector<HTMLElement>('[role="menuitem"]')?.focus({ preventScroll: true });
  }, [x, y]);

  useEffect(() => {
    function onDown(e: MouseEvent) {
      if (!ref.current?.contains(e.target as Node)) onClose();
    }
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") {
        e.preventDefault();
        onClose();
        return;
      }
      if (e.key === "Tab") {
        e.preventDefault();
        onClose();
        return;
      }
      if (e.key !== "ArrowDown" && e.key !== "ArrowUp") return;
      const els = Array.from(ref.current?.querySelectorAll<HTMLElement>('[role="menuitem"]') ?? []);
      if (els.length === 0) return;
      e.preventDefault();
      const idx = els.indexOf(document.activeElement as HTMLElement);
      const next = e.key === "ArrowDown" ? (idx + 1) % els.length : (idx - 1 + els.length) % els.length;
      els[next].focus({ preventScroll: true });
    }
    function onScroll() {
      onClose();
    }
    document.addEventListener("mousedown", onDown);
    document.addEventListener("keydown", onKey);
    window.addEventListener("resize", onScroll);
    window.addEventListener("scroll", onScroll, true);
    return () => {
      document.removeEventListener("mousedown", onDown);
      document.removeEventListener("keydown", onKey);
      window.removeEventListener("resize", onScroll);
      window.removeEventListener("scroll", onScroll, true);
    };
  }, [onClose]);

  return (
    <div
      ref={ref}
      role="menu"
      aria-label={label}
      style={{ left: pos.left, top: pos.top }}
      className="fixed z-[70] w-48 overflow-hidden rounded-md border border-ink-200 bg-ink-0 py-1 shadow-overlay animate-scale-in"
    >
      {items.map((item) => (
        <div key={item.key}>
          {item.separatorBefore && <div role="separator" className="my-1 border-t border-ink-100" />}
          <button
            type="button"
            role="menuitem"
            onClick={() => {
              onClose();
              item.onSelect();
            }}
            className={cn(
              "flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm outline-none",
              item.tone === "danger"
                ? "text-danger-600 hover:bg-danger-50 focus:bg-danger-50"
                : "text-ink-800 hover:bg-ink-100 focus:bg-ink-100"
            )}
          >
            {item.icon}
            {item.label}
          </button>
        </div>
      ))}
    </div>
  );
}
