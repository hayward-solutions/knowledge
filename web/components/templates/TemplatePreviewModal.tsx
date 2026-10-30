"use client";

import type { ReactNode } from "react";
import type { Template } from "@/lib/api";
import { ContentView } from "@/components/editor";
import { Button } from "@/components/ui/Button";
import { Modal } from "@/components/ui/Modal";

export function TemplatePreviewModal({
  template,
  onClose,
  actions,
}: {
  template: Template;
  onClose: () => void;
  actions?: ReactNode;
}) {
  return (
    <Modal
      title={template.name}
      onClose={onClose}
      width={820}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Close
          </Button>
          {actions}
        </>
      }
    >
      {template.description && (
        <p className="mb-4 text-sm text-ink-600">{template.description}</p>
      )}
      <div className="rounded-md border border-ink-200 bg-ink-0 p-4 sm:p-6">
        {template.content_html.trim() ? (
          <ContentView html={template.content_html} />
        ) : (
          <p className="text-sm text-ink-500">This template is empty.</p>
        )}
      </div>
    </Modal>
  );
}
