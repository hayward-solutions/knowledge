import { api, jsonBody } from "./client";
import type { Comment, CommentInput, PMDoc } from "./types";

export function listComments(pageId: string): Promise<Comment[]> {
  return api<Comment[]>(`/pages/${pageId}/comments`);
}

export function createComment(
  pageId: string,
  input: CommentInput
): Promise<Comment> {
  return api<Comment>(`/pages/${pageId}/comments`, {
    method: "POST",
    body: jsonBody(input),
  });
}

export function updateComment(
  id: string,
  input: { body_json: PMDoc | null; body_html: string }
): Promise<Comment> {
  return api<Comment>(`/comments/${id}`, {
    method: "PATCH",
    body: jsonBody(input),
  });
}

export function deleteComment(id: string): Promise<void> {
  return api<void>(`/comments/${id}`, { method: "DELETE" });
}

export function resolveComment(id: string): Promise<Comment> {
  return api<Comment>(`/comments/${id}/resolve`, { method: "POST" });
}

export function reopenComment(id: string): Promise<Comment> {
  return api<Comment>(`/comments/${id}/reopen`, { method: "POST" });
}
