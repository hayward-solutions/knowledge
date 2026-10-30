import { apiUpload, apiUrl } from "./client";
import type { ExportFormat, ImportResult } from "./types";

export function pageExportUrl(pageId: string, format: ExportFormat): string {
  return apiUrl(`/pages/${pageId}/export?format=${format}`);
}

export function spaceExportUrl(key: string): string {
  return apiUrl(`/spaces/${key}/export`);
}

export function importMarkdown(
  key: string,
  files: File[],
  parentId?: string | null
): Promise<ImportResult> {
  const fd = new FormData();
  for (const f of files) fd.append("files", f, f.webkitRelativePath || f.name);
  if (parentId) fd.append("parent_id", parentId);
  return apiUpload<ImportResult>(`/spaces/${key}/import`, fd);
}
