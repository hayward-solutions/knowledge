import { api, jsonBody } from "./client";
import type { Template, TemplateInput } from "./types";

export function listTemplates(): Promise<Template[]> {
  return api<Template[]>("/templates");
}

export function listSpaceTemplates(key: string): Promise<Template[]> {
  return api<Template[]>(`/spaces/${key}/templates`);
}

export function createSpaceTemplate(
  key: string,
  input: TemplateInput
): Promise<Template> {
  return api<Template>(`/spaces/${key}/templates`, {
    method: "POST",
    body: jsonBody(input),
  });
}

export function createGlobalTemplate(input: TemplateInput): Promise<Template> {
  return api<Template>("/admin/templates", {
    method: "POST",
    body: jsonBody(input),
  });
}

export function updateTemplate(
  id: string,
  patch: Partial<TemplateInput>
): Promise<Template> {
  return api<Template>(`/templates/${id}`, {
    method: "PATCH",
    body: jsonBody(patch),
  });
}

export function deleteTemplate(id: string): Promise<void> {
  return api<void>(`/templates/${id}`, { method: "DELETE" });
}
