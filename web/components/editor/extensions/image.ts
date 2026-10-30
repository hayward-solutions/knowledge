import Image from "@tiptap/extension-image";
import { apiUrl } from "@/lib/api/client";

export function resolveImageSrc(src: string | null | undefined): string {
  if (!src) return "";
  return src.startsWith("/api/") ? apiUrl(src) : src;
}

export const EditorImage = Image.extend({
  addAttributes() {
    return {
      ...this.parent?.(),
      width: {
        default: null,
        parseHTML: (element: HTMLElement) => element.getAttribute("width"),
        renderHTML: (attributes: Record<string, unknown>) =>
          attributes.width ? { width: String(attributes.width) } : {},
      },
      attachmentId: {
        default: null,
        parseHTML: (element: HTMLElement) => element.getAttribute("data-attachment-id"),
        renderHTML: (attributes: Record<string, unknown>) =>
          attributes.attachmentId
            ? { "data-attachment-id": attributes.attachmentId as string }
            : {},
      },
    };
  },

  addNodeView() {
    return ({ node }) => {
      const img = document.createElement("img");
      const apply = (attrs: Record<string, unknown>) => {
        img.setAttribute("src", resolveImageSrc(attrs.src as string | null));
        if (attrs.alt) img.setAttribute("alt", String(attrs.alt));
        else img.removeAttribute("alt");
        if (attrs.title) img.setAttribute("title", String(attrs.title));
        else img.removeAttribute("title");
        if (attrs.width) img.setAttribute("width", String(attrs.width));
        else img.removeAttribute("width");
      };
      apply(node.attrs);
      return {
        dom: img,
        update: (updated) => {
          if (updated.type !== node.type) return false;
          apply(updated.attrs);
          return true;
        },
      };
    };
  },
});
