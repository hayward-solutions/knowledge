"use client";

import { Download } from "lucide-react";
import { spaceExportUrl } from "@/lib/api";
import { cn } from "@/lib/cn";

export function ExportSpaceButton({
  spaceKey,
  className,
  label = "Export space",
}: {
  spaceKey: string;
  className?: string;
  label?: string;
}) {
  return (
    <a
      href={spaceExportUrl(spaceKey)}
      download={`${spaceKey}-export.zip`}
      className={cn("btn btn-secondary", className)}
    >
      <Download size={14} />
      {label}
    </a>
  );
}
