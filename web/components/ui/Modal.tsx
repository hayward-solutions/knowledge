"use client";

import { useEffect, useId, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { X } from "lucide-react";

const FOCUSABLE =
  'a[href], button:not([disabled]), textarea:not([disabled]), input:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])';

const modalStack: symbol[] = [];

export function Modal({
  title,
  onClose,
  children,
  width = 640,
  footer,
}: {
  title: string;
  onClose: () => void;
  children: React.ReactNode;
  width?: number;
  footer?: React.ReactNode;
}) {
  const titleId = useId();
  const dialogRef = useRef<HTMLDivElement>(null);
  const onCloseRef = useRef(onClose);
  onCloseRef.current = onClose;

  const [container, setContainer] = useState<HTMLElement | null>(null);
  const previouslyFocused = useRef<HTMLElement | null>(null);

  useEffect(() => {
    previouslyFocused.current = document.activeElement as HTMLElement | null;
    setContainer(document.body);
    return () => {
      const el = previouslyFocused.current;
      if (el && document.contains(el)) el.focus?.();
    };
  }, []);

  useEffect(() => {
    if (!container) return;
    const id = Symbol("modal");
    modalStack.push(id);
    const dialog = dialogRef.current;
    const first = dialog?.querySelector<HTMLElement>(
      "[data-autofocus], input, textarea, select"
    );
    (first ?? dialog)?.focus();

    const onKey = (e: KeyboardEvent) => {
      if (modalStack[modalStack.length - 1] !== id) return;
      if (e.key === "Escape") {
        e.preventDefault();
        onCloseRef.current();
        return;
      }
      if (e.key !== "Tab" || !dialog) return;
      const items = Array.from(dialog.querySelectorAll<HTMLElement>(FOCUSABLE));
      if (items.length === 0) {
        e.preventDefault();
        dialog.focus();
        return;
      }
      const firstEl = items[0];
      const lastEl = items[items.length - 1];
      const active = document.activeElement;
      if (!dialog.contains(active)) {
        e.preventDefault();
        firstEl.focus();
      } else if (e.shiftKey && (active === firstEl || active === dialog)) {
        e.preventDefault();
        lastEl.focus();
      } else if (!e.shiftKey && active === lastEl) {
        e.preventDefault();
        firstEl.focus();
      }
    };
    document.addEventListener("keydown", onKey);
    const prevOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.removeEventListener("keydown", onKey);
      const idx = modalStack.indexOf(id);
      if (idx >= 0) modalStack.splice(idx, 1);
      document.body.style.overflow = prevOverflow;
    };
  }, [container]);

  if (!container) return null;

  return createPortal(
    <div className="fixed inset-0 z-50 flex items-end justify-center p-0 animate-fade-in sm:items-center sm:p-4">
      <div
        className="absolute inset-0 bg-black/50 backdrop-blur-[1px]"
        onClick={onClose}
        aria-hidden
      />
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        tabIndex={-1}
        className="relative z-10 flex max-h-[92vh] w-full flex-col overflow-hidden rounded-t-lg bg-ink-0 shadow-overlay outline-none animate-scale-in sm:max-h-[min(90vh,800px)] sm:rounded-lg"
        style={{ maxWidth: width }}
      >
        <header className="flex shrink-0 items-center justify-between border-b border-ink-200 px-5 py-3">
          <h2 id={titleId} className="text-base font-semibold text-ink-900">
            {title}
          </h2>
          <button
            type="button"
            onClick={onClose}
            className="rounded-xs p-1 text-ink-500 hover:bg-ink-100 hover:text-ink-900"
            aria-label="Close"
          >
            <X size={16} />
          </button>
        </header>
        <div className="flex-1 overflow-y-auto px-5 py-4">{children}</div>
        {footer && (
          <footer className="flex shrink-0 items-center justify-end gap-2 border-t border-ink-200 bg-ink-50 px-5 py-3">
            {footer}
          </footer>
        )}
      </div>
    </div>,
    container
  );
}
