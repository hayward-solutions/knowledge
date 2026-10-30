"use client";

import { FileCode2, FileText } from "lucide-react";
import { pageExportUrl } from "@/lib/api";
import { MenuAnchor } from "./MenuLink";

export function ExportMenu({
  page,
  onSelect,
}: {
  page: { id: string; title?: string };
  onSelect?: () => void;
}) {
  return (
    <>
      <div className="px-3 pb-0.5 pt-1.5 text-[11px] font-semibold uppercase tracking-wide text-ink-500">
        Export
      </div>
      <MenuAnchor href={pageExportUrl(page.id, "md")} download onClick={onSelect}>
        <FileText size={14} className="text-ink-500" />
        Markdown (.md)
      </MenuAnchor>
      <MenuAnchor href={pageExportUrl(page.id, "html")} download onClick={onSelect}>
        <FileCode2 size={14} className="text-ink-500" />
        HTML (.html)
      </MenuAnchor>
    </>
  );
}
