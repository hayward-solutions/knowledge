"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import {
  Bell,
  BookOpen,
  ChevronsUpDown,
  Home,
  LayoutGrid,
  LayoutTemplate,
  Search,
  Shield,
  X,
} from "lucide-react";
import { listSpaces } from "@/lib/api";
import { useMe } from "@/lib/auth";
import { cn } from "@/lib/cn";
import { Avatar } from "@/components/ui/Avatar";
import { Menu, MenuDivider } from "@/components/ui/Dropdown";

function activeSpaceKey(pathname: string): string | null {
  const m = pathname.match(/^\/spaces\/([^/]+)/);
  return m ? decodeURIComponent(m[1]) : null;
}

function SidebarLink({
  href,
  icon,
  active,
  children,
}: {
  href: string;
  icon: React.ReactNode;
  active: boolean;
  children: React.ReactNode;
}) {
  return (
    <Link
      href={href}
      aria-current={active ? "page" : undefined}
      className={cn(
        "flex h-8 items-center gap-2 rounded-md px-2 text-sm font-medium transition",
        active
          ? "bg-brand-50 text-brand-700"
          : "text-ink-700 hover:bg-ink-100 hover:text-ink-900"
      )}
    >
      <span className="flex h-4 w-4 shrink-0 items-center justify-center">{icon}</span>
      <span className="truncate">{children}</span>
    </Link>
  );
}

export function Sidebar({
  mobileOpen = false,
  onMobileClose,
  children,
}: {
  mobileOpen?: boolean;
  onMobileClose?: () => void;
  children?: React.ReactNode;
}) {
  const pathname = usePathname();
  const { data: me } = useMe();
  const spaces = useQuery({ queryKey: ["spaces"], queryFn: listSpaces });
  const key = activeSpaceKey(pathname);
  const active = spaces.data?.find((s) => s.key === key);

  return (
    <>
      <div
        onClick={onMobileClose}
        aria-hidden
        className={cn(
          "fixed inset-0 z-40 bg-black/50 backdrop-blur-[1px] transition-opacity md:hidden",
          mobileOpen ? "opacity-100" : "pointer-events-none opacity-0"
        )}
      />
      <aside
        className={cn(
          "fixed inset-y-0 left-0 z-50 flex h-screen w-[84vw] max-w-[300px] flex-col border-r border-ink-200 bg-ink-0 shadow-drawer transition-transform duration-200 md:sticky md:top-0 md:z-auto md:w-64 md:max-w-none md:shrink-0 md:translate-x-0 md:shadow-none",
          mobileOpen ? "translate-x-0" : "-translate-x-full"
        )}
      >
        <div className="flex h-14 shrink-0 items-center justify-between gap-2 border-b border-ink-200 px-4">
          <Link href="/" className="flex items-center gap-2" onClick={onMobileClose}>
            <span className="flex h-7 w-7 items-center justify-center rounded-md bg-gradient-to-br from-brand-500 to-brand-700 text-white">
              <BookOpen size={16} />
            </span>
            <span className="text-[15px] font-semibold tracking-tight text-ink-900">
              Knowledge
            </span>
          </Link>
          {onMobileClose && (
            <button
              type="button"
              onClick={onMobileClose}
              aria-label="Close navigation"
              className="flex h-9 w-9 items-center justify-center rounded-md text-ink-500 hover:bg-ink-100 hover:text-ink-900 md:hidden"
            >
              <X size={18} />
            </button>
          )}
        </div>

        <div className="shrink-0 border-b border-ink-200 p-2">
          <Menu
            align="left"
            width={248}
            label="Switch space"
            className="block w-full"
            triggerClassName="block w-full"
            trigger={(open) => (
              <span
                className={cn(
                  "flex h-10 w-full items-center gap-2 rounded-md border border-ink-200 px-2 text-left text-sm transition",
                  open ? "bg-ink-100" : "hover:bg-ink-50"
                )}
              >
                {active ? (
                  <>
                    <Avatar name={active.name} seed={active.id} size={22} />
                    <span className="min-w-0 flex-1 truncate font-medium text-ink-900">
                      {active.name}
                    </span>
                  </>
                ) : (
                  <>
                    <LayoutGrid size={16} className="text-ink-500" />
                    <span className="min-w-0 flex-1 truncate text-ink-600">Switch space</span>
                  </>
                )}
                <ChevronsUpDown size={14} className="shrink-0 text-ink-400" />
              </span>
            )}
          >
            {(close) => (
              <div className="max-h-80 overflow-y-auto">
                {spaces.isPending && (
                  <div className="px-3 py-2 text-sm text-ink-500">Loading spaces…</div>
                )}
                {spaces.data?.length === 0 && (
                  <div className="px-3 py-2 text-sm text-ink-500">No spaces yet.</div>
                )}
                {spaces.data?.map((s) => (
                  <Link
                    key={s.id}
                    href={`/spaces/${s.key}`}
                    role="menuitem"
                    onClick={() => {
                      close();
                      onMobileClose?.();
                    }}
                    className={cn(
                      "flex items-center gap-2 px-3 py-1.5 text-sm outline-none hover:bg-ink-100 focus:bg-ink-100",
                      s.key === key ? "text-brand-700" : "text-ink-800"
                    )}
                  >
                    <Avatar name={s.name} seed={s.id} size={20} />
                    <span className="min-w-0 flex-1 truncate">{s.name}</span>
                    <span className="shrink-0 text-[11px] font-semibold text-ink-500">
                      {s.key}
                    </span>
                  </Link>
                ))}
                <MenuDivider />
                <Link
                  href="/spaces"
                  role="menuitem"
                  onClick={() => {
                    close();
                    onMobileClose?.();
                  }}
                  className="flex items-center gap-2 px-3 py-1.5 text-sm text-ink-800 outline-none hover:bg-ink-100 focus:bg-ink-100"
                >
                  <LayoutGrid size={14} className="text-ink-500" />
                  Browse all spaces
                </Link>
              </div>
            )}
          </Menu>
        </div>

        <nav className="shrink-0 space-y-0.5 px-2 py-3" aria-label="Primary">
          <SidebarLink href="/" icon={<Home size={16} />} active={pathname === "/"}>
            Home
          </SidebarLink>
          <SidebarLink
            href="/spaces"
            icon={<LayoutGrid size={16} />}
            active={pathname === "/spaces"}
          >
            Spaces
          </SidebarLink>
          <SidebarLink
            href="/search"
            icon={<Search size={16} />}
            active={pathname.startsWith("/search")}
          >
            Search
          </SidebarLink>
          <SidebarLink
            href="/templates"
            icon={<LayoutTemplate size={16} />}
            active={pathname.startsWith("/templates")}
          >
            Templates
          </SidebarLink>
          <SidebarLink
            href="/notifications"
            icon={<Bell size={16} />}
            active={pathname.startsWith("/notifications")}
          >
            Notifications
          </SidebarLink>
          {me?.is_system_admin && (
            <SidebarLink
              href="/admin/users"
              icon={<Shield size={16} />}
              active={pathname.startsWith("/admin")}
            >
              Admin
            </SidebarLink>
          )}
        </nav>

        {children && (
          <div className="min-h-0 flex-1 overflow-y-auto border-t border-ink-200 px-2 py-3">
            {children}
          </div>
        )}
      </aside>
    </>
  );
}
