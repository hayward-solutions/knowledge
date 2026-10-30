import fs from "node:fs";
import path from "node:path";

function parseEnvFile(file: string): Record<string, string> {
  const out: Record<string, string> = {};
  if (!fs.existsSync(file)) return out;
  for (const raw of fs.readFileSync(file, "utf8").split(/\r?\n/)) {
    const line = raw.trim();
    if (!line || line.startsWith("#")) continue;
    const eq = line.indexOf("=");
    if (eq < 0) continue;
    out[line.slice(0, eq).trim()] = line.slice(eq + 1).trim();
  }
  return out;
}

const root = path.resolve(__dirname, "../../..");
const fileEnv = {
  ...parseEnvFile(path.join(root, ".env.example")),
  ...parseEnvFile(path.join(root, ".env")),
};

export const BASE_URL = process.env.E2E_BASE_URL || "http://localhost:3000";
export const API_URL = (process.env.E2E_API_URL || "http://localhost:8080").replace(/\/$/, "");
export const ADMIN_EMAIL = process.env.E2E_ADMIN_EMAIL || fileEnv.ADMIN_EMAIL || "admin@example.com";
export const ADMIN_PASSWORD =
  process.env.E2E_ADMIN_PASSWORD || fileEnv.ADMIN_PASSWORD || "Change-Me-On-First-Boot-1";
export const STATE_DIR = path.resolve(__dirname, "../.state");
export const ADMIN_STATE = path.join(STATE_DIR, "admin.json");
export const RUN_FILE = path.join(STATE_DIR, "run.json");
export const FIXTURE_DIR = path.resolve(__dirname, "../fixtures");
