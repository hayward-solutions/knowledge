import type { PMDoc } from "@/lib/api/types";

export const EMPTY_DOC: PMDoc = {
  type: "doc",
  content: [{ type: "paragraph" }],
};

export function emptyDoc(): PMDoc {
  return { type: "doc", content: [{ type: "paragraph" }] };
}

interface PMNodeLike {
  type?: string;
  text?: string;
  attrs?: Record<string, unknown>;
  content?: PMNodeLike[];
}

const INLINE_LEAVES = new Set(["mention", "status", "pageLink", "image", "hardBreak"]);

const CONTENT_NODES = new Set([
  "image",
  "horizontalRule",
  "toc",
  "mention",
  "status",
  "pageLink",
  "table",
]);

export function isEmptyDoc(json: PMDoc | null | undefined): boolean {
  if (!json) return true;
  const hasContent = (n: PMNodeLike): boolean => {
    if (n.type === "text") return Boolean(n.text && n.text.trim());
    if (n.type && CONTENT_NODES.has(n.type)) return true;
    return (n.content ?? []).some(hasContent);
  };
  return !hasContent(json as PMNodeLike);
}

function leafLabel(n: PMNodeLike): string {
  const a = n.attrs ?? {};
  switch (n.type) {
    case "mention":
      return `@${String(a.label ?? a.id ?? "")}`;
    case "status":
      return String(a.text ?? "");
    case "pageLink":
      return String(a.title ?? "");
    case "image":
      return String(a.alt ?? "");
    default:
      return "";
  }
}

export function docToPlainText(json: PMDoc | null | undefined): string {
  if (!json) return "";
  const lines: string[] = [];
  let current = "";
  const flush = () => {
    const line = current.replace(/[ \t]+/g, " ").trim();
    if (line) lines.push(line);
    current = "";
  };
  const walk = (n: PMNodeLike) => {
    if (n.type === "text") {
      current += n.text ?? "";
      return;
    }
    if (n.type === "hardBreak") {
      flush();
      return;
    }
    if (n.type && INLINE_LEAVES.has(n.type)) {
      const label = leafLabel(n);
      if (label) current += (current && !current.endsWith(" ") ? " " : "") + label + " ";
      return;
    }
    flush();
    for (const c of n.content ?? []) walk(c);
    flush();
  };
  walk(json as PMNodeLike);
  flush();
  return lines.join("\n");
}
