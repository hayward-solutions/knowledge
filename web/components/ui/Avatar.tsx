import { cn } from "@/lib/cn";

const palette = [
  { bg: "#CCE0FF", fg: "#09326C" },
  { bg: "#BAF3DB", fg: "#164B35" },
  { bg: "#F8E6A0", fg: "#7F5F01" },
  { bg: "#FFD5D2", fg: "#AE2A19" },
  { bg: "#DFD8FD", fg: "#5E4DB2" },
  { bg: "#C6EDFB", fg: "#206B74" },
  { bg: "#FDD0EC", fg: "#AE4787" },
  { bg: "#E4E6EA", fg: "#44546F" },
];

function hash(s: string): number {
  let h = 0;
  for (let i = 0; i < s.length; i++) {
    h = (h << 5) - h + s.charCodeAt(i);
    h |= 0;
  }
  return Math.abs(h);
}

function initials(label: string): string {
  const clean = label.trim();
  if (!clean) return "?";
  const base = clean.includes("@") ? clean.split("@")[0] : clean;
  const parts = base.split(/\s+|[._-]/).filter(Boolean);
  if (parts.length === 0) return clean.slice(0, 2).toUpperCase();
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase();
  return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase();
}

export function Avatar({
  name,
  seed,
  size = 24,
  className,
  title,
}: {
  name: string;
  seed?: string;
  size?: number;
  className?: string;
  title?: string;
}) {
  const k = palette[hash(seed ?? name) % palette.length];
  const fontSize = Math.max(10, Math.round(size * 0.42));
  return (
    <span
      title={title ?? name}
      aria-label={name}
      className={cn(
        "inline-flex shrink-0 select-none items-center justify-center rounded-full font-semibold",
        className
      )}
      style={{
        width: size,
        height: size,
        background: k.bg,
        color: k.fg,
        fontSize,
        lineHeight: 1,
      }}
    >
      {initials(name)}
    </span>
  );
}

export function AvatarGroup({
  names,
  size = 24,
  max = 3,
}: {
  names: string[];
  size?: number;
  max?: number;
}) {
  const visible = names.slice(0, max);
  const extra = names.length - visible.length;
  return (
    <div className="inline-flex items-center">
      {visible.map((n, i) => (
        <span
          key={`${n}-${i}`}
          style={{ marginLeft: i === 0 ? 0 : -6 }}
          className="rounded-full ring-2 ring-ink-0"
        >
          <Avatar name={n} size={size} />
        </span>
      ))}
      {extra > 0 && (
        <span
          style={{ marginLeft: -6, width: size, height: size, fontSize: Math.max(10, size * 0.38) }}
          className="inline-flex items-center justify-center rounded-full bg-ink-100 font-semibold text-ink-700 ring-2 ring-ink-0"
        >
          +{extra}
        </span>
      )}
    </div>
  );
}
