import { ApiError } from "@/lib/api";

export function slugSegment(slug: string | undefined | null): string {
  return slug && slug.length > 0 ? slug : "untitled";
}

export function pageHref(
  spaceKey: string,
  page: { id: string; slug?: string | null }
): string {
  return `/spaces/${spaceKey}/pages/${page.id}/${slugSegment(page.slug)}`;
}

export function pageEditHref(spaceKey: string, id: string): string {
  return `/spaces/${spaceKey}/pages/${id}/edit`;
}

export function pageHistoryHref(spaceKey: string, id: string): string {
  return `/spaces/${spaceKey}/pages/${id}/history`;
}

export function pageAttachmentsHref(spaceKey: string, id: string): string {
  return `/spaces/${spaceKey}/pages/${id}/attachments`;
}

export function errorMessage(err: unknown, fallback: string): string {
  if (err instanceof Error && err.message) return err.message;
  return fallback;
}

export function isApiStatus(err: unknown, ...statuses: number[]): boolean {
  return err instanceof ApiError && statuses.includes(err.status);
}

export function formatBytes(n: number): string {
  if (!Number.isFinite(n) || n < 0) return "";
  if (n < 1024) return `${n} B`;
  const units = ["KB", "MB", "GB", "TB"];
  let v = n / 1024;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v >= 10 ? v.toFixed(0) : v.toFixed(1)} ${units[i]}`;
}

const BLOCK_TAGS = new Set([
  "P",
  "DIV",
  "H1",
  "H2",
  "H3",
  "H4",
  "H5",
  "H6",
  "LI",
  "TR",
  "PRE",
  "BLOCKQUOTE",
  "UL",
  "OL",
  "TABLE",
  "DETAILS",
  "SUMMARY",
  "HR",
  "SECTION",
]);

export function htmlToText(html: string): string {
  if (!html) return "";
  if (typeof DOMParser === "undefined") return html.replace(/<[^>]+>/g, " ");
  const doc = new DOMParser().parseFromString(html, "text/html");
  const out: string[] = [];
  const walk = (node: Node) => {
    if (node.nodeType === Node.TEXT_NODE) {
      out.push(node.textContent ?? "");
      return;
    }
    if (node.nodeType !== Node.ELEMENT_NODE) return;
    const el = node as Element;
    const tag = el.tagName;
    if (tag === "SCRIPT" || tag === "STYLE") return;
    if (tag === "BR") {
      out.push("\n");
      return;
    }
    if (tag === "IMG") {
      const alt = el.getAttribute("alt");
      out.push(`[image${alt ? `: ${alt}` : ""}]`);
      return;
    }
    if (tag === "LI") out.push("• ");
    el.childNodes.forEach(walk);
    if (tag === "TD" || tag === "TH") out.push("\t");
    if (BLOCK_TAGS.has(tag)) out.push("\n");
  };
  walk(doc.body);
  return out
    .join("")
    .replace(/[ \t]+\n/g, "\n")
    .replace(/\n{3,}/g, "\n\n")
    .trim();
}

export function absoluteUrl(url: string): string {
  if (/^https?:\/\//.test(url)) return url;
  if (typeof window === "undefined") return url;
  return `${window.location.origin}${url}`;
}
