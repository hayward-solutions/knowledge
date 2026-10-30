const BASE = "http://safe-next.invalid";
const UNSAFE = /[\u0000-\u001f\u007f-\u009f\\]/;

export function safeNext(raw: string | null | undefined, fallback = "/"): string {
  if (typeof raw !== "string" || raw === "") return fallback;
  if (UNSAFE.test(raw)) return fallback;
  if (!raw.startsWith("/") || raw.startsWith("//")) return fallback;
  let url: URL;
  try {
    url = new URL(raw, BASE);
  } catch {
    return fallback;
  }
  if (url.origin !== BASE) return fallback;
  const out = `${url.pathname}${url.search}${url.hash}`;
  if (!out.startsWith("/") || out.startsWith("//") || UNSAFE.test(out)) return fallback;
  return out;
}
