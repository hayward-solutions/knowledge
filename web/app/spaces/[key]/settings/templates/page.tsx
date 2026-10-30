"use client";

import { useParams } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { TemplateManager } from "@/components/templates/TemplateManager";
import { getSpace } from "@/lib/api";
import { useMe } from "@/lib/auth";
import { EmptyState } from "@/components/ui/EmptyState";
import { Spinner } from "@/components/ui/Spinner";

export default function SpaceTemplatesPage() {
  const params = useParams<{ key: string }>();
  const key = params.key;
  const { data: me } = useMe();
  const space = useQuery({
    queryKey: ["templates", "space-meta", key],
    queryFn: () => getSpace(key),
  });

  const canManage =
    Boolean(me?.is_system_admin) || space.data?.role === "admin" || space.data?.role === "editor";

  return (
    <div className="mx-auto flex w-full max-w-4xl flex-col gap-4 p-4 sm:p-6">
      <h1 className="text-[20px] font-semibold tracking-tight text-ink-900">Templates</h1>
      {space.isPending ? (
        <div className="flex justify-center py-16">
          <Spinner size={20} />
        </div>
      ) : space.isError ? (
        <EmptyState title="Space unavailable" description={space.error.message} />
      ) : (
        <TemplateManager scope={{ kind: "space", key }} canManage={canManage} />
      )}
    </div>
  );
}
