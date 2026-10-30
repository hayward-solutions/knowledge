import { api } from "./client";
import type { Page, PageVersion, PageVersionSummary } from "./types";

export function listVersions(pageId: string): Promise<PageVersionSummary[]> {
  return api<PageVersionSummary[]>(`/pages/${pageId}/versions`);
}

export function getVersion(pageId: string, version: number): Promise<PageVersion> {
  return api<PageVersion>(`/pages/${pageId}/versions/${version}`);
}

export function restoreVersion(pageId: string, version: number): Promise<Page> {
  return api<Page>(`/pages/${pageId}/versions/${version}/restore`, {
    method: "POST",
  });
}
