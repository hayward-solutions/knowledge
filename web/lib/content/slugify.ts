export function slugify(input: string): string {
  const base = input
    .normalize("NFKD")
    .replace(/[̀-ͯ]/g, "")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 80)
    .replace(/-+$/g, "");
  if (!base) return "section";
  return /^[a-z]/.test(base) ? base : `h-${base}`;
}

export function uniqueSlug(base: string, taken: Set<string>): string {
  let candidate = base;
  let n = 1;
  while (taken.has(candidate)) {
    candidate = `${base}-${n}`;
    n += 1;
  }
  taken.add(candidate);
  return candidate;
}

export const HEADING_ID_PATTERN = /^[A-Za-z][A-Za-z0-9_\-:.]{0,99}$/;
