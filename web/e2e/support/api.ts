import { request, type APIRequestContext } from "@playwright/test";
import { ADMIN_STATE, API_URL, BASE_URL } from "./env";

export interface ApiPage {
  id: string;
  title: string;
  slug: string;
  version: number;
  space_key?: string;
}

export interface ApiUser {
  id: string;
  email: string;
  display_name: string;
}

export async function loginContext(email: string, password: string): Promise<APIRequestContext> {
  const ctx = await request.newContext({
    baseURL: `${API_URL}/api/v1/`,
    extraHTTPHeaders: { Origin: BASE_URL },
  });
  const res = await ctx.post("auth/login", { data: { email, password } });
  if (!res.ok()) throw new Error(`login ${email} failed: ${res.status()} ${await res.text()}`);
  return ctx;
}

export async function adminApi(): Promise<APIRequestContext> {
  return request.newContext({
    baseURL: `${API_URL}/api/v1/`,
    extraHTTPHeaders: { Origin: BASE_URL },
    storageState: ADMIN_STATE,
  });
}

async function must<T>(res: Awaited<ReturnType<APIRequestContext["get"]>>, what: string): Promise<T> {
  if (!res.ok()) throw new Error(`${what} failed: ${res.status()} ${await res.text()}`);
  if (res.status() === 204) return undefined as T;
  return (await res.json()) as T;
}

export async function createUser(
  api: APIRequestContext,
  email: string,
  displayName: string,
  password: string
): Promise<ApiUser> {
  return must<ApiUser>(
    await api.post("admin/users", {
      data: { email, display_name: displayName, password },
    }),
    "create user"
  );
}

export async function addMember(
  api: APIRequestContext,
  spaceKey: string,
  userId: string,
  role: "admin" | "editor" | "viewer"
) {
  return must(await api.put(`spaces/${spaceKey}/members/${userId}`, { data: { role } }), "add member");
}

export async function createPublishedPage(
  api: APIRequestContext,
  spaceKey: string,
  title: string,
  html: string,
  parentId?: string
): Promise<ApiPage> {
  const page = await must<ApiPage>(
    await api.post(`spaces/${spaceKey}/pages`, {
      data: { title, parent_id: parentId },
    }),
    "create page"
  );
  return must<ApiPage>(
    await api.post(`pages/${page.id}/publish`, {
      data: {
        title,
        content_json: null,
        content_html: html,
        base_version: page.version,
        message: "Created by e2e",
      },
    }),
    "publish page"
  );
}

export async function getPage(api: APIRequestContext, id: string) {
  return must<Record<string, unknown>>(await api.get(`pages/${id}`), "get page");
}

export async function getSpace(api: APIRequestContext, key: string) {
  const res = await api.get(`spaces/${key}`);
  if (res.status() === 404) return null;
  return must<Record<string, unknown>>(res, "get space");
}

export async function unreadCount(api: APIRequestContext): Promise<number> {
  const res = await api.get("notifications?unread=true&limit=1");
  const header = res.headers()["x-unread-count"];
  return header ? Number(header) : 0;
}

export async function ensureSpace(api: APIRequestContext, key: string, name = "Engineering") {
  const existing = await getSpace(api, key);
  if (existing) return existing;
  return must<Record<string, unknown>>(
    await api.post("spaces", { data: { key, name, description: "", visibility: "internal" } }),
    "create space"
  );
}

export async function setLabels(api: APIRequestContext, pageId: string, labels: string[]) {
  return must(await api.put(`pages/${pageId}/labels`, { data: { labels } }), "set labels");
}
