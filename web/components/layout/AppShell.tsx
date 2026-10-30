"use client";

import { useEffect, useState, type ReactNode } from "react";
import { usePathname } from "next/navigation";
import { RequireAuth } from "@/lib/auth";
import { cn } from "@/lib/cn";
import { Sidebar } from "@/components/layout/Sidebar";
import { TopBar } from "@/components/layout/TopBar";

export interface AppShellProps {
  children: ReactNode;
  sidebar?: ReactNode;
  topSlot?: ReactNode;
  contentClassName?: string;
}

function Frame({ children, sidebar, topSlot, contentClassName }: AppShellProps) {
  const [mobileNavOpen, setMobileNavOpen] = useState(false);
  const pathname = usePathname();

  useEffect(() => {
    setMobileNavOpen(false);
  }, [pathname]);

  useEffect(() => {
    if (!mobileNavOpen) return;
    const prev = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = prev;
    };
  }, [mobileNavOpen]);

  return (
    <div className="flex min-h-screen bg-ink-50">
      <Sidebar mobileOpen={mobileNavOpen} onMobileClose={() => setMobileNavOpen(false)}>
        {sidebar}
      </Sidebar>
      <div className="flex min-w-0 flex-1 flex-col">
        <TopBar onMenuClick={() => setMobileNavOpen(true)}>{topSlot}</TopBar>
        <main className={cn("flex min-h-0 flex-1 flex-col", contentClassName)}>{children}</main>
      </div>
    </div>
  );
}

export function AppShell(props: AppShellProps) {
  return (
    <RequireAuth>
      <Frame {...props} />
    </RequireAuth>
  );
}
