"use client";

import type { EditorChange } from "@/components/editor";
import { Modal } from "@/components/ui/Modal";
import { CommentComposer } from "./CommentComposer";

export function InlineCommentDialog({
  quote,
  onClose,
  onSubmit,
}: {
  quote: string;
  onClose: () => void;
  onSubmit: (value: EditorChange) => Promise<boolean>;
}) {
  return (
    <Modal title="Add inline comment" onClose={onClose} width={520}>
      <div className="space-y-3">
        <blockquote className="max-h-32 overflow-y-auto rounded-xs border-l-2 border-warning-500 bg-warning-50 px-3 py-2 text-sm text-ink-700">
          {quote}
        </blockquote>
        <CommentComposer
          autofocus
          placeholder="Write a comment on this text"
          submitLabel="Comment"
          ariaLabel="Inline comment"
          onCancel={onClose}
          onSubmit={onSubmit}
        />
      </div>
    </Modal>
  );
}
