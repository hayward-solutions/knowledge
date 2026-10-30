import Link from "next/link";
import { ChevronRight } from "lucide-react";
import type { PageSummary, Space } from "@/lib/api";
import { pageHref } from "./utils";

export function PageBreadcrumbs({
  space,
  ancestors,
  title,
}: {
  space: Pick<Space, "key" | "name" | "home_page_id">;
  ancestors: PageSummary[];
  title?: string;
}) {
  const trail = ancestors.filter((a) => a.id !== space.home_page_id);
  return (
    <nav aria-label="Breadcrumb" className="min-w-0 text-sm text-ink-600">
      <ol className="flex flex-wrap items-center gap-x-1 gap-y-0.5">
        <li className="flex items-center gap-1">
          <Link href="/spaces" className="hover:text-ink-900 hover:underline">
            Spaces
          </Link>
          <ChevronRight size={12} className="text-ink-400" aria-hidden />
        </li>
        <li className="flex items-center gap-1">
          <Link href={`/spaces/${space.key}`} className="max-w-[200px] truncate hover:text-ink-900 hover:underline">
            {space.name}
          </Link>
          {(trail.length > 0 || title) && <ChevronRight size={12} className="text-ink-400" aria-hidden />}
        </li>
        {trail.map((a, i) => (
          <li key={a.id} className="flex items-center gap-1">
            <Link
              href={pageHref(space.key, a)}
              className="max-w-[200px] truncate hover:text-ink-900 hover:underline"
            >
              {a.title || "Untitled"}
            </Link>
            {(i < trail.length - 1 || title) && (
              <ChevronRight size={12} className="text-ink-400" aria-hidden />
            )}
          </li>
        ))}
        {title && (
          <li aria-current="page" className="max-w-[260px] truncate font-medium text-ink-900">
            {title}
          </li>
        )}
      </ol>
    </nav>
  );
}
