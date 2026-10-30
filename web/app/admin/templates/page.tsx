"use client";

import { TemplateManager } from "@/components/templates/TemplateManager";

export default function AdminTemplatesPage() {
  return (
    <>
      <div className="border-b border-ink-200 bg-ink-0 px-4 py-4 sm:px-6">
        <h2 className="text-base font-semibold text-ink-900">Global templates</h2>
        <p className="text-sm text-ink-600">
          Create and edit starting points for pages in any space.
        </p>
      </div>
      <div className="flex-1 bg-ink-0 px-4 py-4 sm:px-6">
        <TemplateManager scope={{ kind: "global" }} canManage />
      </div>
    </>
  );
}
