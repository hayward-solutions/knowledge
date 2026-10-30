"use client";

import { AppShell } from "@/components/layout/AppShell";
import { useMe } from "@/lib/auth";

export default function HomePage() {
  const { data: me } = useMe();
  return (
    <AppShell>
      <div className="border-b border-ink-200 bg-ink-0 px-4 py-4 sm:px-6">
        <h1 className="text-[20px] font-semibold tracking-tight text-ink-900">
          Welcome{me ? `, ${me.display_name || me.email}` : ""}
        </h1>
        <p className="text-sm text-ink-600">
          Your recent updates, drafts, and starred pages will appear here.
        </p>
      </div>
    </AppShell>
  );
}
