import { api, apiUpload, apiUrl } from "./client";
import type { Attachment } from "./types";

export function listAttachments(pageId: string): Promise<Attachment[]> {
  return api<Attachment[]>(`/pages/${pageId}/attachments`);
}

export function uploadAttachment(pageId: string, file: File): Promise<Attachment> {
  const fd = new FormData();
  fd.append("file", file, file.name);
  return apiUpload<Attachment>(`/pages/${pageId}/attachments`, fd);
}

export function deleteAttachment(id: string): Promise<void> {
  return api<void>(`/attachments/${id}`, { method: "DELETE" });
}

export function attachmentContentPath(id: string): string {
  return `/api/v1/attachments/${id}/content`;
}

export function attachmentContentUrl(id: string): string {
  return apiUrl(attachmentContentPath(id));
}

export function attachmentDownloadUrl(id: string): string {
  return apiUrl(`/attachments/${id}`);
}
