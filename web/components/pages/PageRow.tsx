import Link from "next/link";
import { FileText } from "lucide-react";
import type { DashboardPage } from "@/lib/api";
import { Badge } from "@/components/ui/Badge";
import { RelativeTime } from "./RelativeTime";
import { pageEditHref, pageHref } from "./utils";

export function PageRow({ page, edit = false }: { page: DashboardPage; edit?: boolean }) {
  const href = edit ? pageEditHref(page.space_key, page.id) : pageHref(page.space_key, page);
  return (
    <li>
      <Link
        href={href}
        className="flex items-center gap-3 px-4 py-2.5 text-sm outline-none transition hover:bg-ink-50 focus-visible:bg-ink-50"
      >
        <FileText size={16} className="shrink-0 text-ink-400" />
        <span className="min-w-0 flex-1">
          <span className="block truncate font-medium text-ink-900">{page.title || "Untitled"}</span>
          {page.author && (
            <span className="block truncate text-xs text-ink-600">{page.author.display_name}</span>
          )}
        </span>
        {page.status === "draft" && <Badge tone="amber">Draft</Badge>}
        <span className="chip shrink-0 bg-ink-100 text-ink-700" title={page.space_name}>
          {page.space_key}
        </span>
        <RelativeTime iso={page.updated_at} className="hidden shrink-0 whitespace-nowrap text-right text-xs text-ink-600 sm:block" />
      </Link>
    </li>
  );
}
