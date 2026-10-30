import type { Editor as TiptapEditor } from "@tiptap/core";
import type { PMDoc, UserRef } from "@/lib/api/types";

export interface EditorValue {
  json: PMDoc | null;
  html: string;
}

export interface EditorChange {
  json: PMDoc;
  html: string;
  text: string;
}

export interface UploadedImage {
  id: string;
  url: string;
}

export type UploadImageFn = (file: File) => Promise<UploadedImage>;
export type SearchUsersFn = (q: string) => Promise<UserRef[]>;
export type SearchPagesFn = (q: string) => Promise<PageLinkItem[]>;

export interface PageLinkItem {
  id: string;
  title: string;
  slug: string;
  spaceKey: string;
  spaceName?: string;
}

export interface CommentSelection {
  from: number;
  to: number;
  text: string;
}

export interface EditorHandle {
  editor: TiptapEditor | null;
  getValue: () => EditorChange | null;
  setContent: (value: EditorValue) => void;
  clear: () => void;
  focus: (position?: "start" | "end") => void;
  isEmpty: () => boolean;
}

export function pageLinkHref(item: Pick<PageLinkItem, "id" | "slug" | "spaceKey">): string {
  return `/spaces/${item.spaceKey}/pages/${item.id}/${item.slug}`;
}
