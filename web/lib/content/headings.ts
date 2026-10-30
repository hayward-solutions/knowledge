import { HEADING_ID_PATTERN, slugify, uniqueSlug } from "./slugify";

export interface HeadingInfo {
  id: string;
  level: number;
  text: string;
}

const ENTITIES: Record<string, string> = {
  amp: "&",
  lt: "<",
  gt: ">",
  quot: '"',
  apos: "'",
  nbsp: " ",
};

function decodeEntities(s: string): string {
  return s.replace(/&(#x[0-9a-f]+|#\d+|[a-z]+);/gi, (m, body: string) => {
    if (body[0] === "#") {
      const code =
        body[1] === "x" || body[1] === "X"
          ? parseInt(body.slice(2), 16)
          : parseInt(body.slice(1), 10);
      if (!Number.isFinite(code) || code <= 0 || code > 0x10ffff) return m;
      return String.fromCodePoint(code);
    }
    return ENTITIES[body.toLowerCase()] ?? m;
  });
}

function stripTags(s: string): string {
  return decodeEntities(s.replace(/<[^>]*>/g, "")).replace(/\s+/g, " ").trim();
}

const HEADING_RE = /<h([1-6])((?:\s[^>]*)?)>([\s\S]*?)<\/h\1\s*>/gi;
const ID_ATTR_RE = /\sid\s*=\s*(?:"([^"]*)"|'([^']*)')/i;

export function resolveHeadingIds(
  entries: { id: string | null; text: string }[]
): string[] {
  const taken = new Set<string>();
  for (const e of entries) {
    if (e.id && HEADING_ID_PATTERN.test(e.id)) taken.add(e.id);
  }
  const used = new Set<string>();
  return entries.map((e) => {
    if (e.id && HEADING_ID_PATTERN.test(e.id) && !used.has(e.id)) {
      used.add(e.id);
      return e.id;
    }
    const base = slugify(e.text);
    const merged = new Set<string>([...taken, ...used]);
    const id = uniqueSlug(base, merged);
    used.add(id);
    return id;
  });
}

export function extractHeadings(html: string): HeadingInfo[] {
  if (!html) return [];
  const raw: { level: number; id: string | null; text: string }[] = [];
  HEADING_RE.lastIndex = 0;
  let m: RegExpExecArray | null;
  while ((m = HEADING_RE.exec(html)) !== null) {
    const idMatch = ID_ATTR_RE.exec(m[2] ?? "");
    raw.push({
      level: Number(m[1]),
      id: idMatch ? decodeEntities(idMatch[1] ?? idMatch[2] ?? "") : null,
      text: stripTags(m[3]),
    });
  }
  const ids = resolveHeadingIds(raw);
  return raw.map((h, i) => ({ id: ids[i], level: h.level, text: h.text }));
}
