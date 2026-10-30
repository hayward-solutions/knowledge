import Link from "next/link";

export function Breadcrumbs({
  items,
}: {
  items: { label: string; href?: string }[];
}) {
  return (
    <nav aria-label="Breadcrumb" className="flex items-center gap-1.5 text-sm text-ink-600">
      {items.map((it, i) => (
        <span key={i} className="flex items-center gap-1.5">
          {i > 0 && <span className="text-ink-300">/</span>}
          {it.href ? (
            <Link href={it.href} className="hover:text-ink-900 hover:underline">
              {it.label}
            </Link>
          ) : (
            <span className="font-semibold text-ink-900">{it.label}</span>
          )}
        </span>
      ))}
    </nav>
  );
}
