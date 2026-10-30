import Link from "next/link";
import { Archive, FileText, Lock, Globe } from "lucide-react";
import type { Role, Space } from "@/lib/api";
import { Badge, type BadgeTone } from "@/components/ui/Badge";
import { SpaceIcon } from "./SpaceIcon";

const roleTone: Record<Role, BadgeTone> = {
  admin: "purple",
  editor: "blue",
  viewer: "neutral",
};

export function SpaceCard({ space }: { space: Space }) {
  const archived = !!space.archived_at;
  return (
    <Link
      href={`/spaces/${space.key}`}
      className="surface card-hover group flex h-full flex-col gap-3 p-4 outline-none focus-visible:ring-2 focus-visible:ring-brand-400"
    >
      <div className="flex items-start gap-3">
        <SpaceIcon space={space} size={40} />
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <h2 className="truncate text-[15px] font-semibold text-ink-900 group-hover:underline">
              {space.name}
            </h2>
          </div>
          <div className="mt-0.5 flex items-center gap-1.5">
            <span className="chip bg-ink-100 text-ink-700">{space.key}</span>
            {archived && (
              <Badge tone="amber">
                <Archive size={10} />
                Archived
              </Badge>
            )}
          </div>
        </div>
      </div>
      <p className="line-clamp-2 min-h-[2.5rem] text-sm text-ink-600">
        {space.description || "No description"}
      </p>
      <div className="mt-auto flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-ink-600">
        <span className="inline-flex items-center gap-1">
          <FileText size={12} />
          {space.page_count ?? 0} {space.page_count === 1 ? "page" : "pages"}
        </span>
        <span className="inline-flex items-center gap-1">
          {space.visibility === "private" ? <Lock size={12} /> : <Globe size={12} />}
          {space.visibility === "private" ? "Private" : "Internal"}
        </span>
        {space.role && (
          <Badge tone={roleTone[space.role]} className="ml-auto">
            {space.role}
          </Badge>
        )}
      </div>
    </Link>
  );
}
