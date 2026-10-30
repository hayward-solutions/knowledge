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

const INLINE_WRAP: Record<string, [string, string]> = {
  STRONG: ["**", "**"],
  B: ["**", "**"],
  EM: ["_", "_"],
  I: ["_", "_"],
  U: ["<u>", "</u>"],
  S: ["~~", "~~"],
  DEL: ["~~", "~~"],
  STRIKE: ["~~", "~~"],
  CODE: ["`", "`"],
  MARK: ["==", "=="],
  SUB: ["~", "~"],
  SUP: ["^", "^"],
};

function squash(s: string): string {
  return s.replace(/\s+/g, " ");
}

function inlineStructure(node: Node): string {
  if (node.nodeType === Node.TEXT_NODE) return squash(node.textContent ?? "");
  if (node.nodeType !== Node.ELEMENT_NODE) return "";
  const el = node as Element;
  const tag = el.tagName;
  if (tag === "BR") return "↵";
  if (tag === "SCRIPT" || tag === "STYLE") return "";
  if (tag === "IMG") {
    const alt = el.getAttribute("alt") ?? "";
    const width = el.getAttribute("width");
    return `[image: ${alt}${width ? ` ${width}` : ""}]`;
  }
  const inner = Array.from(el.childNodes).map(inlineStructure).join("");
  const type = el.getAttribute("data-type");
  if (tag === "A") {
    if (type === "page-link") return `[[${inner}]]`;
    return `[${inner}](${el.getAttribute("href") ?? ""})`;
  }
  if (type === "mention") return inner.startsWith("@") ? inner : `@${inner}`;
  if (type === "status") return `{status:${el.getAttribute("data-color") ?? ""}|${inner}}`;
  const wrap = INLINE_WRAP[tag];
  if (wrap) return `${wrap[0]}${inner}${wrap[1]}`;
  if (BLOCK_TAGS.has(tag)) return inner;
  return inner;
}

function textOfBlocks(el: Element): string {
  return squash(
    Array.from(el.childNodes)
      .map((c) => {
        if (c.nodeType === Node.ELEMENT_NODE && /^(UL|OL)$/.test((c as Element).tagName)) return "";
        return inlineStructure(c);
      })
      .join(" ")
  ).trim();
}

export function htmlToStructure(html: string): string {
  if (!html) return "";
  if (typeof DOMParser === "undefined") return html;
  const doc = new DOMParser().parseFromString(html, "text/html");
  const lines: string[] = [];

  const join = (label: string | null, tag: string) => (label ? `${label}/${tag}` : tag);
  const align = (el: Element) => {
    const a = el.getAttribute("data-align");
    return a ? `[${a}]` : "";
  };

  const emitList = (list: Element, depth: number, label: string | null) => {
    const ordered = list.tagName === "OL";
    const indent = "  ".repeat(depth);
    Array.from(list.children).forEach((li) => {
      if (li.tagName !== "LI") return;
      const checked = li.getAttribute("data-checked");
      let name = ordered ? "ol-li" : "li";
      if (checked !== null) name += checked === "true" ? "[x]" : "[ ]";
      const own = textOfBlocks(li);
      lines.push(`${indent}${label ? `${label}/` : ""}${name}| ${own}`);
      Array.from(li.children).forEach((c) => {
        if (c.tagName === "UL" || c.tagName === "OL") emitList(c, depth + 1, label);
        else if (c.tagName === "PRE" || c.tagName === "TABLE" || c.tagName === "DIV") walk(c, label);
      });
    });
  };

  const flushInline = (buf: Node[], label: string | null) => {
    if (buf.length === 0) return;
    const text = squash(buf.map(inlineStructure).join("")).trim();
    buf.length = 0;
    if (text) lines.push(`${label ?? "p"}| ${text}`);
  };

  function walk(container: Element, label: string | null) {
    const buf: Node[] = [];
    container.childNodes.forEach((child) => {
      if (child.nodeType === Node.TEXT_NODE) {
        buf.push(child);
        return;
      }
      if (child.nodeType !== Node.ELEMENT_NODE) return;
      const el = child as Element;
      const tag = el.tagName;
      const type = el.getAttribute("data-type");
      if (!BLOCK_TAGS.has(tag) && tag !== "TD" && tag !== "TH" && tag !== "TBODY" && tag !== "THEAD") {
        buf.push(el);
        return;
      }
      flushInline(buf, label);
      if (/^H[1-6]$/.test(tag)) {
        lines.push(`${join(label, tag.toLowerCase())}${align(el)}| ${textOfBlocks(el)}`);
      } else if (tag === "P") {
        lines.push(`${label ?? "p"}${align(el)}| ${textOfBlocks(el)}`);
      } else if (tag === "UL" || tag === "OL") {
        emitList(el, 0, label);
      } else if (tag === "PRE") {
        const lang = Array.from(el.querySelector("code")?.classList ?? [])
          .find((c) => c.startsWith("language-"))
          ?.slice("language-".length);
        const name = `${join(label, "code")}${lang ? `:${lang}` : ""}`;
        (el.textContent ?? "").replace(/\n$/, "").split("\n").forEach((l) => lines.push(`${name}| ${l}`));
      } else if (tag === "HR") {
        lines.push(`${join(label, "hr")}|`);
      } else if (tag === "TABLE") {
        el.querySelectorAll("tr").forEach((tr) => {
          const cells = Array.from(tr.children).filter((c) => c.tagName === "TD" || c.tagName === "TH");
          const header = cells.length > 0 && cells.every((c) => c.tagName === "TH");
          const rendered = cells.map((c) => {
            const span = Number(c.getAttribute("colspan") ?? "1");
            const text = squash(Array.from(c.childNodes).map(inlineStructure).join(" ")).trim();
            return span > 1 ? `${text} {colspan=${span}}` : text;
          });
          lines.push(`${join(label, header ? "table-header" : "table-row")}| ${rendered.join(" | ")}`);
        });
      } else if (type === "toc") {
        lines.push(`${join(label, "toc")}|`);
      } else if (type === "panel") {
        walk(el, join(label, `panel:${el.getAttribute("data-color") ?? ""}`));
      } else if (tag === "DETAILS") {
        const summary = el.querySelector(":scope > summary");
        lines.push(`${join(label, "expand")}| ${summary ? textOfBlocks(summary) : ""}`);
        Array.from(el.children).forEach((c) => {
          if (c.tagName !== "SUMMARY") walk(c, join(label, "expand"));
        });
      } else if (tag === "BLOCKQUOTE") {
        walk(el, join(label, "quote"));
      } else {
        walk(el, label);
      }
    });
    flushInline(buf, label);
  }

  walk(doc.body, null);
  return lines.join("\n");
}
