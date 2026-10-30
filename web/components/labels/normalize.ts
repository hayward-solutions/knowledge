export const MAX_LABELS_PER_PAGE = 20;
export const MAX_LABEL_LENGTH = 50;

export function normalizeLabel(raw: string): string {
  return raw.trim().toLowerCase().replace(/\s+/g, "-");
}

export function labelError(name: string): string | null {
  if (!name) return "Enter a label name";
  if (name.length > MAX_LABEL_LENGTH) return `Labels can be at most ${MAX_LABEL_LENGTH} characters`;
  if (!/^[a-z0-9._-]+$/.test(name)) {
    return "Use letters, numbers, dashes, underscores and dots only";
  }
  return null;
}
