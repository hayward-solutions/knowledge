import type { PMDoc } from "@/lib/api/types";

export type InitialContent = PMDoc | string;

export function htmlToInitialContent(value: {
  json: PMDoc | null | undefined;
  html: string | null | undefined;
}): InitialContent {
  if (value.json && typeof value.json === "object") return value.json;
  return value.html ?? "";
}
