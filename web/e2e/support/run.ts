import fs from "node:fs";
import { RUN_FILE } from "./env";

export interface RunInfo {
  spaceKey: string;
  runId: string;
}

const workerTag = Math.random().toString(36).slice(2, 6);

let cached: RunInfo | null = null;

export function run(): RunInfo {
  if (!cached) {
    const base = JSON.parse(fs.readFileSync(RUN_FILE, "utf8")) as RunInfo;
    cached = { spaceKey: base.spaceKey, runId: `${base.runId}${workerTag}` };
  }
  return cached;
}

export function spaceKey(): string {
  return run().spaceKey;
}

export function uniq(prefix: string): string {
  return `${prefix} ${run().runId}`;
}
