"use client";

import type { ReactNode } from "react";
import Link from "next/link";
import { Lock } from "lucide-react";
import { EmptyState } from "@/components/ui/EmptyState";
import { Tabs, type TabItem } from "@/components/ui/Tabs";
import { useSpace } from "./SpaceContext";

export type SettingsTab = "general" | "members" | "templates" | "import-export";

export function SettingsShell({
  active,
  children,
  adminOnly = true,
}: {
  active: SettingsTab;
  children: ReactNode;
  adminOnly?: boolean;
}) {
  const { space, isAdmin } = useSpace();
  const base = `/spaces/${space.key}/settings`;
  const tabs: TabItem<SettingsTab>[] = [
    { id: "general", label: "General", href: `${base}/general` },
    { id: "members", label: "Members", href: `${base}/members` },
    { id: "templates", label: "Templates", href: `${base}/templates` },
    { id: "import-export", label: "Import and export", href: `${base}/import-export` },
  ];

  if (adminOnly && !isAdmin) {
    return (
      <div className="mx-auto w-full max-w-xl px-4 py-16">
        <EmptyState
          icon={<Lock size={18} />}
          title="Space settings are for admins"
          description="Ask a space admin if you need something changed."
          action={
            <Link href={`/spaces/${space.key}`} className="btn btn-primary">
              Back to space
            </Link>
          }
        />
      </div>
    );
  }

  return (
    <div className="flex-1 bg-ink-0">
      <div className="px-4 pt-6 sm:px-8">
        <div className="mx-auto w-full max-w-4xl">
          <h1 className="text-[22px] font-semibold tracking-tight text-ink-900">
            Space settings
            <span className="ml-2 text-base font-medium text-ink-500">{space.name}</span>
          </h1>
        </div>
      </div>
      <div className="mt-3 px-4 sm:px-8">
        <div className="mx-auto w-full max-w-4xl">
          <Tabs tabs={tabs} value={active} />
        </div>
      </div>
      <div className="px-4 py-6 sm:px-8">
        <div className="mx-auto w-full max-w-4xl">{children}</div>
      </div>
    </div>
  );
}
