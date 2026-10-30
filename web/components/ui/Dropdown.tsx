"use client";

import { useCallback, useEffect, useId, useRef, useState } from "react";
import { cn } from "@/lib/cn";

export function Menu({
  trigger,
  children,
  align = "right",
  width = 200,
  label,
  className,
  triggerClassName,
}: {
  trigger: (open: boolean) => React.ReactNode;
  children: (close: () => void) => React.ReactNode;
  align?: "left" | "right";
  width?: number;
  label?: string;
  className?: string;
  triggerClassName?: string;
}) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const panelRef = useRef<HTMLDivElement>(null);
  const panelId = useId();

  const close = useCallback(() => {
    setOpen(false);
    triggerRef.current?.focus();
  }, []);

  useEffect(() => {
    if (!open) return;
    function onDocClick(e: MouseEvent) {
      if (!ref.current?.contains(e.target as Node)) setOpen(false);
    }
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") {
        close();
        return;
      }
      if (e.key !== "ArrowDown" && e.key !== "ArrowUp") return;
      const items = Array.from(
        panelRef.current?.querySelectorAll<HTMLElement>('[role="menuitem"]:not([disabled])') ?? []
      );
      if (items.length === 0) return;
      e.preventDefault();
      const idx = items.indexOf(document.activeElement as HTMLElement);
      const next =
        e.key === "ArrowDown"
          ? items[(idx + 1) % items.length]
          : items[(idx - 1 + items.length) % items.length];
      next.focus();
    }
    document.addEventListener("mousedown", onDocClick);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDocClick);
      document.removeEventListener("keydown", onKey);
    };
  }, [open, close]);

  return (
    <div className={cn("relative inline-block", className)} ref={ref}>
      <button
        ref={triggerRef}
        type="button"
        aria-haspopup="menu"
        aria-expanded={open}
        aria-controls={open ? panelId : undefined}
        aria-label={label}
        onClick={() => setOpen((o) => !o)}
        className={cn("rounded-md outline-none focus-visible:ring-2 focus-visible:ring-brand-400", triggerClassName)}
      >
        {trigger(open)}
      </button>
      {open && (
        <div
          id={panelId}
          ref={panelRef}
          role="menu"
          style={{ width, [align]: 0 }}
          className="absolute z-50 mt-1 overflow-hidden rounded-md border border-ink-200 bg-ink-0 py-1 shadow-overlay animate-scale-in"
        >
          {children(() => setOpen(false))}
        </div>
      )}
    </div>
  );
}

export function MenuItem({
  onClick,
  children,
  tone = "default",
  disabled,
}: {
  onClick?: () => void;
  children: React.ReactNode;
  tone?: "default" | "danger";
  disabled?: boolean;
}) {
  return (
    <button
      type="button"
      role="menuitem"
      disabled={disabled}
      onClick={onClick}
      className={cn(
        "flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm outline-none disabled:opacity-50",
        tone === "danger"
          ? "text-danger-600 hover:bg-danger-50 focus:bg-danger-50"
          : "text-ink-800 hover:bg-ink-100 focus:bg-ink-100"
      )}
    >
      {children}
    </button>
  );
}

export function MenuDivider() {
  return <div role="separator" className="my-1 border-t border-ink-100" />;
}

export const Dropdown = Menu;
