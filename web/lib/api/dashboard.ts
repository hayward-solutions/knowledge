import { api } from "./client";
import type { Dashboard } from "./types";

export function getDashboard(): Promise<Dashboard> {
  return api<Dashboard>("/me/dashboard");
}
