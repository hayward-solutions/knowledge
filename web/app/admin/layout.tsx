"use client";

import { usePathname } from "next/navigation";
import { AppShell } from "@/components/layout/AppShell";
import { Breadcrumbs } from "@/components/layout/Breadcrumbs";
import { RequireAdmin } from "@/lib/auth";
import { Tabs, type TabItem } from "@/components/ui/Tabs";

const TABS: TabItem[] = [
  { id: "users", label: "Users", href: "/admin/users" },
  { id: "spaces", label: "Spaces", href: "/admin/spaces" },
  { id: "templates", label: "Templates", href: "/admin/templates" },
  { id: "labels", label: "Labels", href: "/admin/labels" },
];

export default function AdminLayout({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const active = TABS.find((t) => pathname.startsWith(t.href ?? ""))?.id ?? "users";
  const label = TABS.find((t) => t.id === active)?.label ?? "Users";

  return (
    <AppShell
      topSlot={<Breadcrumbs items={[{ label: "Admin", href: "/admin/users" }, { label }]} />}
    >
      <RequireAdmin>
        <div className="bg-ink-0">
          <div className="px-4 pt-4 sm:px-6">
            <h1 className="text-[20px] font-semibold tracking-tight text-ink-900">
              Administration
            </h1>
          </div>
          <Tabs tabs={TABS} value={active} className="mt-3 px-4 sm:px-6" />
        </div>
        <div className="flex min-h-0 flex-1 flex-col">{children}</div>
      </RequireAdmin>
    </AppShell>
  );
}
