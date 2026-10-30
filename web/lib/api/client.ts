export const API_BASE = process.env.NEXT_PUBLIC_API_BASE_URL ?? "";

export class ApiError extends Error {
  status: number;
  code?: string;
  body?: unknown;
  constructor(message: string, status: number, code?: string, body?: unknown) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.body = body;
  }
}

export function apiUrl(path: string): string {
  if (/^https?:\/\//.test(path)) return path;
  if (path.startsWith("/api/")) return `${API_BASE}${path}`;
  return `${API_BASE}/api/v1${path}`;
}

async function toError(res: Response): Promise<ApiError> {
  let message = res.statusText || `Request failed (${res.status})`;
  let code: string | undefined;
  let body: unknown;
  try {
    body = await res.json();
    if (body && typeof body === "object") {
      const b = body as { error?: unknown; code?: unknown };
      if (typeof b.error === "string" && b.error) message = b.error;
      if (typeof b.code === "string") code = b.code;
    }
  } catch {
    body = undefined;
  }
  return new ApiError(message, res.status, code, body);
}

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const { headers, ...rest } = init;
  const hasBody = rest.body !== undefined && rest.body !== null;
  const res = await fetch(apiUrl(path), {
    credentials: "include",
    ...rest,
    headers: {
      Accept: "application/json",
      ...(hasBody ? { "Content-Type": "application/json" } : {}),
      ...(headers || {}),
    },
  });
  if (!res.ok) throw await toError(res);
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

export async function apiUpload<T>(
  path: string,
  formData: FormData,
  init: Omit<RequestInit, "body"> = {}
): Promise<T> {
  const { headers, ...rest } = init;
  const res = await fetch(apiUrl(path), {
    method: "POST",
    credentials: "include",
    ...rest,
    headers: { Accept: "application/json", ...(headers || {}) },
    body: formData,
  });
  if (!res.ok) throw await toError(res);
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

export function jsonBody(value: unknown): string {
  return JSON.stringify(value);
}

export function qs(params: Record<string, string | number | boolean | null | undefined>): string {
  const sp = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v === undefined || v === null || v === "") continue;
    sp.set(k, String(v));
  }
  const s = sp.toString();
  return s ? `?${s}` : "";
}
