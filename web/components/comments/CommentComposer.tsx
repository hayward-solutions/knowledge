"use client";

import { useRef, useState, type KeyboardEvent } from "react";
import { Editor, type EditorChange, type EditorHandle } from "@/components/editor";
import { Button } from "@/components/ui/Button";
import { cn } from "@/lib/cn";
import { isEmptyDoc } from "@/lib/content";
import type { PMDoc } from "@/lib/api/types";

export interface CommentComposerProps {
  initial?: { json: PMDoc | null; html: string };
  placeholder?: string;
  submitLabel?: string;
  autofocus?: boolean;
  disabled?: boolean;
  disabledMessage?: string;
  onSubmit: (value: EditorChange) => Promise<boolean>;
  onCancel?: () => void;
  className?: string;
  ariaLabel?: string;
}

export function CommentComposer({
  initial,
  placeholder = "Write a comment. Type @ to mention someone.",
  submitLabel = "Comment",
  autofocus = false,
  disabled = false,
  disabledMessage,
  onSubmit,
  onCancel,
  className,
  ariaLabel = "Comment",
}: CommentComposerProps) {
  const ref = useRef<EditorHandle>(null);
  const [empty, setEmpty] = useState(!initial);
  const [busy, setBusy] = useState(false);

  async function submit() {
    const handle = ref.current;
    if (!handle || disabled || busy || handle.isEmpty()) return;
    const value = handle.getValue();
    if (!value) return;
    setBusy(true);
    let ok = false;
    try {
      ok = await onSubmit(value);
    } finally {
      setBusy(false);
    }
    if (ok && !initial) {
      handle.clear();
      setEmpty(true);
    }
  }

  function onKeyDown(e: KeyboardEvent<HTMLDivElement>) {
    if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
      e.preventDefault();
      void submit();
    }
  }

  return (
    <div className={cn("space-y-2", className)} onKeyDown={onKeyDown}>
      <Editor
        ref={ref}
        variant="compact"
        value={initial ?? { json: null, html: "" }}
        placeholder={placeholder}
        editable={!disabled}
        autofocus={autofocus ? "end" : false}
        ariaLabel={ariaLabel}
        onChange={(change) => setEmpty(isEmptyDoc(change.json))}
        className={cn(disabled && "opacity-60")}
      />
      {disabled && disabledMessage && (
        <p className="text-xs text-ink-500">{disabledMessage}</p>
      )}
      {!disabled && (
        <div className="flex items-center justify-end gap-2">
          {onCancel && (
            <Button variant="ghost" size="sm" onClick={onCancel} disabled={busy}>
              Cancel
            </Button>
          )}
          <Button
            variant="primary"
            size="sm"
            onClick={() => void submit()}
            disabled={busy || empty}
          >
            {busy ? "Saving..." : submitLabel}
          </Button>
        </div>
      )}
    </div>
  );
}
