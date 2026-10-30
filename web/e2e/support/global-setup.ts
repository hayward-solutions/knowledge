import fs from "node:fs";
import { request, type FullConfig } from "@playwright/test";
import { ADMIN_EMAIL, ADMIN_PASSWORD, ADMIN_STATE, API_URL, BASE_URL, RUN_FILE, STATE_DIR } from "./env";

async function waitFor(url: string, label: string, timeoutMs = 120_000) {
  const deadline = Date.now() + timeoutMs;
  let last = "";
  while (Date.now() < deadline) {
    try {
      const res = await fetch(url);
      if (res.ok) return;
      last = `status ${res.status}`;
    } catch (e) {
      last = e instanceof Error ? e.message : String(e);
    }
    await new Promise((r) => setTimeout(r, 1000));
  }
  throw new Error(`${label} at ${url} not ready: ${last}`);
}

export default async function globalSetup(_config: FullConfig) {
  await waitFor(`${API_URL}/health`, "api");
  await waitFor(`${BASE_URL}/login`, "web");

  fs.mkdirSync(STATE_DIR, { recursive: true });

  const ctx = await request.newContext({
    baseURL: `${API_URL}/api/v1/`,
    extraHTTPHeaders: { Origin: BASE_URL },
  });
  const login = await ctx.post("auth/login", {
    data: { email: ADMIN_EMAIL, password: ADMIN_PASSWORD },
  });
  if (!login.ok()) {
    throw new Error(`admin login failed (${login.status()}): ${await login.text()}`);
  }
  await ctx.storageState({ path: ADMIN_STATE });

  const taken = await ctx.get("spaces/ENG");
  const suffix = Math.random().toString(36).slice(2, 6).toUpperCase().replace(/[^A-Z0-9]/g, "X");
  const spaceKey = taken.status() === 404 ? "ENG" : `ENG${suffix}`;
  const runId = Date.now().toString(36);
  fs.writeFileSync(RUN_FILE, JSON.stringify({ spaceKey, runId }));
  await ctx.dispose();
}
