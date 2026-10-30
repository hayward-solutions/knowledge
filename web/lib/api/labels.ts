import { api, jsonBody, qs } from "./client";
import type { Label, PageSummary } from "./types";

export function listLabels(q?: string): Promise<Label[]> {
  return api<Label[]>(`/labels${qs({ q })}`);
}

export function getPageLabels(pageId: string): Promise<Label[]> {
  return api<Label[]>(`/pages/${pageId}/labels`);
}

export function setPageLabels(pageId: string, names: string[]): Promise<Label[]> {
  return api<Label[]>(`/pages/${pageId}/labels`, {
    method: "PUT",
    body: jsonBody({ labels: names }),
  });
}

export function listSpaceLabels(key: string): Promise<Label[]> {
  return api<Label[]>(`/spaces/${key}/labels`);
}

export function listLabelPages(key: string, name: string): Promise<PageSummary[]> {
  return api<PageSummary[]>(
    `/spaces/${key}/labels/${encodeURIComponent(name)}/pages`
  );
}
