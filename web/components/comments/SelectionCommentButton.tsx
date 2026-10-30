"use client";

import { useEffect, useRef, useState, type RefObject } from "react";
import { createPortal } from "react-dom";
import { MessageSquarePlus } from "lucide-react";
import { captureContext } from "./anchors";
import { MAX_QUOTE_LENGTH, normalizeQuote } from "./commentUtils";

export interface SelectionRequest {
  quote: string;
  prefix: string;
  suffix: string;
}

interface Placement extends SelectionRequest {
  top: number;
  left: number;
}

const IGNORED = 'pre, [data-type="toc"], .kn-toc, button, input, textarea, [contenteditable="true"]';

export function SelectionCommentButton({
  contentRef,
  enabled,
  onRequest,
}: {
  contentRef?: RefObject<HTMLElement>;
  enabled: boolean;
  onRequest: (request: SelectionRequest) => void;
}) {
  const [placement, setPlacement] = useState<Placement | null>(null);
  const draggingRef = useRef(false);

  useEffect(() => {
    if (!enabled) {
      setPlacement(null);
      return;
    }
    let frame = 0;

    const compute = () => {
      frame = 0;
      const root = contentRef?.current;
      const selection = window.getSelection();
      if (!root || !selection || selection.rangeCount === 0 || selection.isCollapsed) {
        setPlacement(null);
        return;
      }
      const range = selection.getRangeAt(0);
      const container =
        range.commonAncestorContainer.nodeType === Node.ELEMENT_NODE
          ? (range.commonAncestorContainer as Element)
          : range.commonAncestorContainer.parentElement;
      if (!container || !root.contains(container) || container.closest(IGNORED)) {
        setPlacement(null);
        return;
      }
      const text = normalizeQuote(selection.toString()).slice(0, MAX_QUOTE_LENGTH);
      if (!text) {
        setPlacement(null);
        return;
      }
      const rect = range.getBoundingClientRect();
      if (rect.width === 0 && rect.height === 0) {
        setPlacement(null);
        return;
      }
      const narrow = window.innerWidth < 640;
      const above = !narrow && rect.top > 84;
      const top = above ? rect.top - 40 : rect.bottom + 10;
      const left = Math.min(Math.max(rect.left + rect.width / 2, 64), window.innerWidth - 64);
      const context = captureContext(root, range, text);
      setPlacement({
        top,
        left,
        quote: text,
        prefix: context?.prefix ?? "",
        suffix: context?.suffix ?? "",
      });
    };

    const schedule = () => {
      if (draggingRef.current) return;
      if (frame) cancelAnimationFrame(frame);
      frame = requestAnimationFrame(compute);
    };

    const onDown = (event: MouseEvent) => {
      const target = event.target as HTMLElement | null;
      if (target?.closest("[data-selection-comment]")) return;
      draggingRef.current = true;
      setPlacement(null);
    };

    const onUp = () => {
      draggingRef.current = false;
      schedule();
    };

    document.addEventListener("selectionchange", schedule);
    document.addEventListener("mousedown", onDown);
    document.addEventListener("mouseup", onUp);
    window.addEventListener("scroll", schedule, true);
    window.addEventListener("resize", schedule);
    return () => {
      if (frame) cancelAnimationFrame(frame);
      document.removeEventListener("selectionchange", schedule);
      document.removeEventListener("mousedown", onDown);
      document.removeEventListener("mouseup", onUp);
      window.removeEventListener("scroll", schedule, true);
      window.removeEventListener("resize", schedule);
    };
  }, [contentRef, enabled]);

  if (!placement || !enabled) return null;

  return createPortal(
    <button
      type="button"
      data-selection-comment
      onMouseDown={(e) => e.preventDefault()}
      onClick={() => {
        const { quote, prefix, suffix } = placement;
        window.getSelection()?.removeAllRanges();
        setPlacement(null);
        onRequest({ quote, prefix, suffix });
      }}
      style={{ position: "fixed", top: placement.top, left: placement.left, transform: "translateX(-50%)" }}
      className="btn btn-primary z-40 shadow-overlay animate-scale-in"
    >
      <MessageSquarePlus size={14} />
      Comment
    </button>,
    document.body
  );
}
