import { ApiError, api, jsonBody } from "./client";
import type {
  CreatePageInput,
  DraftInput,
  EditingState,
  MovePageInput,
  Page,
  PageDetail,
  PageDraft,
  PageSummary,
  PublishInput,
  VersionConflict,
} from "./types";

export function createPage(key: string, input: CreatePageInput): Promise<Page> {
  return api<Page>(`/spaces/${key}/pages`, {
    method: "POST",
    body: jsonBody(input),
  });
}

export function getPage(id: string): Promise<PageDetail> {
  return api<PageDetail>(`/pages/${id}`);
}

export function updatePage(
  id: string,
  patch: { title?: string }
): Promise<Page> {
  return api<Page>(`/pages/${id}`, { method: "PATCH", body: jsonBody(patch) });
}

export function saveDraft(id: string, input: DraftInput): Promise<PageDraft> {
  return api<PageDraft>(`/pages/${id}/draft`, {
    method: "PUT",
    body: jsonBody(input),
  });
}

export function getDraft(id: string): Promise<PageDraft> {
  return api<PageDraft>(`/pages/${id}/draft`);
}

export function discardDraft(id: string): Promise<void> {
  return api<void>(`/pages/${id}/draft`, { method: "DELETE" });
}

export function publishPage(id: string, input: PublishInput): Promise<Page> {
  return api<Page>(`/pages/${id}/publish`, {
    method: "POST",
    body: jsonBody(input),
  });
}

export function isVersionConflict(err: unknown): err is ApiError & {
  body: VersionConflict;
} {
  return (
    err instanceof ApiError &&
    err.status === 409 &&
    err.code === "version_conflict"
  );
}

export function movePage(id: string, input: MovePageInput): Promise<Page> {
  return api<Page>(`/pages/${id}/move`, {
    method: "POST",
    body: jsonBody(input),
  });
}

export function trashPage(id: string): Promise<void> {
  return api<void>(`/pages/${id}/trash`, { method: "POST" });
}

export function restorePage(id: string): Promise<Page> {
  return api<Page>(`/pages/${id}/restore`, { method: "POST" });
}

export function purgePage(id: string): Promise<void> {
  return api<void>(`/pages/${id}`, { method: "DELETE" });
}

export function copyPage(
  id: string,
  input: { title?: string; parent_id?: string | null } = {}
): Promise<Page> {
  return api<Page>(`/pages/${id}/copy`, {
    method: "POST",
    body: jsonBody(input),
  });
}

export function listChildren(id: string): Promise<PageSummary[]> {
  return api<PageSummary[]>(`/pages/${id}/children`);
}

export function starPage(id: string): Promise<void> {
  return api<void>(`/pages/${id}/star`, { method: "POST" });
}

export function unstarPage(id: string): Promise<void> {
  return api<void>(`/pages/${id}/star`, { method: "DELETE" });
}

export function sendEditingHeartbeat(id: string): Promise<EditingState> {
  return api<EditingState>(`/pages/${id}/editing`, { method: "POST" });
}
