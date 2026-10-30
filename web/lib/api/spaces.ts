import { api, jsonBody, qs } from "./client";
import type {
  Activity,
  CreateSpaceInput,
  PageSummary,
  Role,
  Space,
  SpaceMember,
  TrashedPage,
  UpdateSpaceInput,
} from "./types";

export function listSpaces(): Promise<Space[]> {
  return api<Space[]>("/spaces");
}

export function createSpace(input: CreateSpaceInput): Promise<Space> {
  return api<Space>("/spaces", { method: "POST", body: jsonBody(input) });
}

export function getSpace(key: string): Promise<Space> {
  return api<Space>(`/spaces/${key}`);
}

export function updateSpace(
  key: string,
  patch: UpdateSpaceInput
): Promise<Space> {
  return api<Space>(`/spaces/${key}`, {
    method: "PATCH",
    body: jsonBody(patch),
  });
}

export function deleteSpace(key: string): Promise<void> {
  return api<void>(`/spaces/${key}`, { method: "DELETE" });
}

export function archiveSpace(key: string): Promise<Space> {
  return api<Space>(`/spaces/${key}/archive`, { method: "POST" });
}

export function unarchiveSpace(key: string): Promise<Space> {
  return api<Space>(`/spaces/${key}/unarchive`, { method: "POST" });
}

export function listSpaceMembers(key: string): Promise<SpaceMember[]> {
  return api<SpaceMember[]>(`/spaces/${key}/members`);
}

export function putSpaceMember(
  key: string,
  userId: string,
  role: Role
): Promise<SpaceMember> {
  return api<SpaceMember>(`/spaces/${key}/members/${userId}`, {
    method: "PUT",
    body: jsonBody({ role }),
  });
}

export function removeSpaceMember(key: string, userId: string): Promise<void> {
  return api<void>(`/spaces/${key}/members/${userId}`, { method: "DELETE" });
}

export function getSpaceTree(key: string): Promise<PageSummary[]> {
  return api<PageSummary[]>(`/spaces/${key}/tree`);
}

export function getSpaceActivity(
  key: string,
  params: { before?: string; limit?: number } = {}
): Promise<Activity[]> {
  return api<Activity[]>(`/spaces/${key}/activity${qs(params)}`);
}

export function getSpaceTrash(key: string): Promise<TrashedPage[]> {
  return api<TrashedPage[]>(`/spaces/${key}/trash`);
}
