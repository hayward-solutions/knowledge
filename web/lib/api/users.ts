import { api, qs } from "./client";
import type { UserRef } from "./types";

export function searchUsers(q: string): Promise<UserRef[]> {
  return api<UserRef[]>(`/users${qs({ q })}`);
}

export function getUser(id: string): Promise<UserRef> {
  return api<UserRef>(`/users/${id}`);
}
