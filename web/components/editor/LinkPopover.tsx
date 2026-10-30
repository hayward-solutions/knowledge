"use client";

import type { Editor } from "@tiptap/core";
import { ExternalLink, Link2Off } from "lucide-react";
import { useEffect, useLayoutEffect, useRef, useState } from "react";

export interface LinkPopoverState {
  top: number;
  left: number;
  href: string;
  hasSelection: boolean;
  hasLink: boolean;
}

export function normalizeHref(raw: string): string | null {
  const value = raw.trim();
  if (!value) return null;
  if (/^(https?:\/\/|mailto:|\/|#)/i.test(value)) return value;
  if (/^[a-z][a-z0-9+.-]*:/i.test(value)) return null;
  if (/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value)) return `mailto:${value}`;
  return `https://${value}`;
}

export function computeLinkPopover(editor: Editor): LinkPopoverState {
  const { from, to, empty } = editor.state.selection;
  const coords = editor.view.coordsAtPos(from);
  const attrs = editor.getAttributes("link");
  return {
    top: coords.bottom + 8,
    left: coords.left,
    href: (attrs.href as string | undefined) ?? "",
    hasSelection: !empty && to > from,
    hasLink: editor.isActive("link"),
  };
}

export function LinkPopover({
  editor,
  state,
  onClose,
}: {
  editor: Editor;
  state: LinkPopoverState;
  onClose: () => void;
}) {
  const [href, setHref] = useState(state.href);
  const [text, setText] = useState("");
  const [error, setError] = useState<string | null>(null);
  const rootRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const [pos, setPos] = useState({ top: state.top, left: state.left });

  useLayoutEffect(() => {
    const el = rootRef.current;
    if (!el) return;
    const width = el.offsetWidth;
    const height = el.offsetHeight;
    const left = Math.max(8, Math.min(state.left, window.innerWidth - width - 8));
    let top = state.top;
    if (top + height > window.innerHeight - 8) top = Math.max(8, state.top - height - 32);
    setPos({ top, left });
  }, [state.left, state.top]);

  useEffect(() => {
    inputRef.current?.focus();
    inputRef.current?.select();
    const onDown = (e: MouseEvent) => {
      if (!rootRef.current?.contains(e.target as Node)) onClose();
    };
    document.addEventListener("mousedown", onDown);
    return () => document.removeEventListener("mousedown", onDown);
  }, [onClose]);

  function close() {
    onClose();
    editor.commands.focus();
  }

  function apply() {
    const normalized = normalizeHref(href);
    if (!normalized) {
      setError("Enter a valid http, https or mailto address");
      return;
    }
    const chain = editor.chain().focus();
    if (state.hasSelection || state.hasLink) {
      chain.extendMarkRange("link").setLink({ href: normalized }).run();
    } else {
      chain
        .insertContent({
          type: "text",
          text: text.trim() || normalized.replace(/^mailto:/, ""),
          marks: [{ type: "link", attrs: { href: normalized } }],
        })
        .run();
    }
    onClose();
  }

  function remove() {
    editor.chain().focus().extendMarkRange("link").unsetLink().run();
    onClose();
  }

  const needsText = !state.hasSelection && !state.hasLink;

  return (
    <div
      ref={rootRef}
      className="kn-link-popover"
      style={{ top: pos.top, left: pos.left }}
      role="dialog"
      aria-label="Edit link"
      onKeyDown={(e) => {
        if (e.key === "Escape") {
          e.preventDefault();
          e.stopPropagation();
          close();
        }
      }}
    >
      {needsText && (
        <input
          className="kn-link-input"
          placeholder="Text to display"
          value={text}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              inputRef.current?.focus();
            }
          }}
        />
      )}
      <div className="kn-link-row">
        <input
          ref={inputRef}
          className="kn-link-input"
          placeholder="Paste or type a link"
          value={href}
          onChange={(e) => {
            setHref(e.target.value);
            setError(null);
          }}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              apply();
            }
          }}
          aria-invalid={error ? true : undefined}
        />
        <button type="button" className="kn-link-apply" onClick={apply}>
          {state.hasLink ? "Update" : "Link"}
        </button>
        {state.hasLink && (
          <>
            <a
              className="kn-tb-btn"
              href={state.href}
              target="_blank"
              rel="noopener noreferrer"
              aria-label="Open link"
              title="Open link"
            >
              <ExternalLink size={14} />
            </a>
            <button
              type="button"
              className="kn-tb-btn"
              onClick={remove}
              aria-label="Remove link"
              title="Remove link"
            >
              <Link2Off size={14} />
            </button>
          </>
        )}
      </div>
      {error && <div className="kn-link-error">{error}</div>}
    </div>
  );
}
