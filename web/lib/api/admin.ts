import { api, jsonBody } from "./client";
import type { AdminSpace, User } from "./types";

export interface CreateAdminUserInput {
  email: string;
  password: string;
  display_name: string;
  is_system_admin: boolean;
}

export interface PatchAdminUserInput {
  display_name?: string;
  email?: string;
  password?: string;
  is_system_admin?: boolean;
  disabled?: boolean;
}

export function listAdminUsers(): Promise<User[]> {
  return api<User[]>("/admin/users");
}

export function createAdminUser(input: CreateAdminUserInput): Promise<User> {
  return api<User>("/admin/users", { method: "POST", body: jsonBody(input) });
}

export function patchAdminUser(
  id: string,
  patch: PatchAdminUserInput
): Promise<User> {
  return api<User>(`/admin/users/${id}`, {
    method: "PATCH",
    body: jsonBody(patch),
  });
}

export function listAdminSpaces(): Promise<AdminSpace[]> {
  return api<AdminSpace[]>("/admin/spaces");
}
