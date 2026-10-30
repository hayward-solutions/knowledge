import Link from "next/link";
import { FileText } from "lucide-react";
import type { PageSummary } from "@/lib/api";
import { Badge } from "@/components/ui/Badge";
import { pageHref } from "./utils";

export function ChildPages({ spaceKey, pages }: { spaceKey: string; pages: PageSummary[] }) {
  if (pages.length === 0) return null;
  const sorted = [...pages].sort((a, b) => a.position - b.position || a.title.localeCompare(b.title));
  return (
    <section aria-labelledby="child-pages-heading" className="mt-10 print:hidden">
      <h2 id="child-pages-heading" className="mb-3 text-sm font-semibold text-ink-900">
        Child pages
      </h2>
      <ul className="grid gap-2 sm:grid-cols-2">
        {sorted.map((p) => (
          <li key={p.id}>
            <Link
              href={pageHref(spaceKey, p)}
              className="surface card-hover flex items-center gap-2 px-3 py-2.5 text-sm text-ink-900"
            >
              <FileText size={16} className="shrink-0 text-ink-400" />
              <span className="min-w-0 flex-1 truncate font-medium">{p.title || "Untitled"}</span>
              {p.status === "draft" && <Badge tone="amber">Draft</Badge>}
              {p.has_children && <span className="shrink-0 text-xs text-ink-500">has children</span>}
            </Link>
          </li>
        ))}
      </ul>
    </section>
  );
}
