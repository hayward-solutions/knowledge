"use client";

import { useMemo, useState } from "react";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { FileText, Settings } from "lucide-react";
import { AppShell } from "@/components/layout/AppShell";
import { Breadcrumbs } from "@/components/layout/Breadcrumbs";
import { TemplatePreviewModal } from "@/components/templates/TemplatePreviewModal";
import { UseTemplateDialog } from "@/components/templates/UseTemplateDialog";
import { listSpaces, listTemplates, type Template } from "@/lib/api";
import { useMe } from "@/lib/auth";
import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";
import { Spinner } from "@/components/ui/Spinner";

interface Group {
  id: string;
  title: string;
  manageHref: string | null;
  templates: Template[];
}

function TemplateCard({
  template,
  onPreview,
  onUse,
}: {
  template: Template;
  onPreview: () => void;
  onUse: () => void;
}) {
  return (
    <li className="surface card-hover flex flex-col rounded-lg border border-ink-200 p-4">
      <div className="flex items-start gap-3">
        <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md bg-brand-50 text-brand-700">
          <FileText size={18} />
        </span>
        <div className="min-w-0">
          <h3 className="truncate text-sm font-semibold text-ink-900">{template.name}</h3>
          <p className="mt-0.5 line-clamp-3 text-sm text-ink-600">
            {template.description || "No description"}
          </p>
        </div>
      </div>
      <div className="mt-4 flex items-center justify-end gap-2">
        <Button size="sm" variant="ghost" onClick={onPreview}>
          Preview
        </Button>
        <Button size="sm" variant="primary" onClick={onUse}>
          Use template
        </Button>
      </div>
    </li>
  );
}

export default function TemplatesPage() {
  const { data: me } = useMe();
  const templates = useQuery({ queryKey: ["templates"], queryFn: listTemplates });
  const spaces = useQuery({ queryKey: ["spaces"], queryFn: listSpaces, staleTime: 60_000 });
  const [preview, setPreview] = useState<Template | null>(null);
  const [using, setUsing] = useState<Template | null>(null);

  const groups = useMemo<Group[]>(() => {
    const byId = new Map((spaces.data ?? []).map((s) => [s.id, s]));
    const global: Template[] = [];
    const bySpace = new Map<string, Template[]>();
    for (const t of templates.data ?? []) {
      if (t.space_id === null) {
        global.push(t);
        continue;
      }
      const list = bySpace.get(t.space_id) ?? [];
      list.push(t);
      bySpace.set(t.space_id, list);
    }
    const out: Group[] = [];
    if (global.length > 0) {
      out.push({
        id: "global",
        title: "Global templates",
        manageHref: me?.is_system_admin ? "/admin/templates" : null,
        templates: global,
      });
    }
    const spaceGroups = Array.from(bySpace.entries()).map(([spaceId, list]) => {
      const space = byId.get(spaceId);
      const key = space?.key ?? list[0].space_key ?? "";
      return {
        id: spaceId,
        title: space ? `${space.name} (${space.key})` : key || "Space templates",
        manageHref: key ? `/spaces/${key}/settings/templates` : null,
        templates: list,
      };
    });
    spaceGroups.sort((a, b) => a.title.localeCompare(b.title));
    return [...out, ...spaceGroups];
  }, [templates.data, spaces.data, me?.is_system_admin]);

  return (
    <AppShell topSlot={<Breadcrumbs items={[{ label: "Templates" }]} />}>
      <div className="mx-auto flex w-full max-w-5xl flex-col gap-6 p-4 sm:p-6">
        <div>
          <h1 className="text-[20px] font-semibold tracking-tight text-ink-900">Templates</h1>
          <p className="mt-1 text-sm text-ink-600">
            Start a new page from a ready-made structure.
          </p>
        </div>

        {templates.isPending ? (
          <div className="flex justify-center py-16">
            <Spinner size={20} />
          </div>
        ) : templates.isError ? (
          <p role="alert" className="text-sm text-danger-600">
            {templates.error.message}
          </p>
        ) : groups.length === 0 ? (
          <EmptyState
            icon={<FileText size={18} />}
            title="No templates available"
            description="Space admins and editors can create templates from a space's settings."
          />
        ) : (
          groups.map((g) => (
            <section key={g.id} aria-labelledby={`tpl-${g.id}`} className="flex flex-col gap-3">
              <div className="flex items-center justify-between gap-3">
                <div className="flex items-center gap-2">
                  <h2 id={`tpl-${g.id}`} className="text-sm font-semibold text-ink-900">
                    {g.title}
                  </h2>
                  <Badge tone={g.id === "global" ? "purple" : "neutral"}>
                    {g.templates.length}
                  </Badge>
                </div>
                {g.manageHref && (
                  <Link
                    href={g.manageHref}
                    className="inline-flex items-center gap-1 text-xs font-medium text-brand-700 hover:underline"
                  >
                    <Settings size={12} /> Manage
                  </Link>
                )}
              </div>
              <ul className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
                {g.templates.map((t) => (
                  <TemplateCard
                    key={t.id}
                    template={t}
                    onPreview={() => setPreview(t)}
                    onUse={() => setUsing(t)}
                  />
                ))}
              </ul>
            </section>
          ))
        )}
      </div>

      {preview && (
        <TemplatePreviewModal
          template={preview}
          onClose={() => setPreview(null)}
          actions={
            <Button
              variant="primary"
              onClick={() => {
                setUsing(preview);
                setPreview(null);
              }}
            >
              Use template
            </Button>
          }
        />
      )}
      {using && <UseTemplateDialog template={using} onClose={() => setUsing(null)} />}
    </AppShell>
  );
}
