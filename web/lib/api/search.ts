import { api, qs } from "./client";
import type { QuickSearchHit, SearchParams, SearchResult } from "./types";

export function search(params: SearchParams): Promise<SearchResult> {
  return api<SearchResult>(`/search${qs({ ...params })}`);
}

export function quickSearch(q: string): Promise<QuickSearchHit[]> {
  return api<QuickSearchHit[]>(`/search/quick${qs({ q })}`);
}
