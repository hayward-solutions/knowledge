import { safeNext } from "../safe-next";
import { api, apiUrl, jsonBody } from "./client";
import type { OIDCConfig, User } from "./types";

export function login(email: string, password: string): Promise<User> {
  return api<User>("/auth/login", {
    method: "POST",
    body: jsonBody({ email, password }),
  });
}

export function logout(): Promise<void> {
  return api<void>("/auth/logout", { method: "POST" });
}

export function getMe(): Promise<User> {
  return api<User>("/auth/me");
}

export function updateMe(patch: {
  display_name?: string;
  email?: string;
}): Promise<User> {
  return api<User>("/auth/me", { method: "PATCH", body: jsonBody(patch) });
}

export function changePassword(
  currentPassword: string,
  newPassword: string
): Promise<void> {
  return api<void>("/auth/change-password", {
    method: "POST",
    body: jsonBody({
      current_password: currentPassword,
      new_password: newPassword,
    }),
  });
}

export function oidcConfig(): Promise<OIDCConfig> {
  return api<OIDCConfig>("/auth/oidc/config");
}

export function oidcLoginUrl(next?: string): string {
  const q = next ? `?next=${encodeURIComponent(safeNext(next))}` : "";
  return apiUrl(`/auth/oidc/login${q}`);
}
