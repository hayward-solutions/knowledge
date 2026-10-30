"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useQueryClient } from "@tanstack/react-query";
import { ChevronDown, LogOut, Menu as MenuIcon, Moon, Plus, Settings, Sun } from "lucide-react";
import { logout } from "@/lib/api";
import { useMe } from "@/lib/auth";
import { useTheme } from "@/lib/theme";
import { Avatar } from "@/components/ui/Avatar";
import { Menu, MenuDivider, MenuItem } from "@/components/ui/Dropdown";
import { useToast } from "@/components/ui/Toast";
import { QuickSearch } from "@/components/search/QuickSearch";
import { NotificationBell } from "@/components/notifications/NotificationBell";

const APP_VERSION = process.env.NEXT_PUBLIC_APP_VERSION || "dev";

export function TopBar({
  children,
  onMenuClick,
}: {
  children?: React.ReactNode;
  onMenuClick?: () => void;
}) {
  const router = useRouter();
  const qc = useQueryClient();
  const toast = useToast();
  const { resolved, toggle } = useTheme();
  const { data: me } = useMe();

  async function signOut() {
    try {
      await logout();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Could not sign out");
      return;
    }
    qc.clear();
    router.push("/login");
  }

  const label = me?.display_name || me?.email || "Account";

  return (
    <header className="sticky top-0 z-30 flex h-14 shrink-0 items-center gap-2 border-b border-ink-200 bg-ink-0/95 px-2 backdrop-blur sm:gap-3 sm:px-6">
      {onMenuClick && (
        <button
          type="button"
          onClick={onMenuClick}
          aria-label="Open navigation"
          className="flex h-10 w-10 shrink-0 items-center justify-center rounded-md text-ink-700 hover:bg-ink-100 md:hidden"
        >
          <MenuIcon size={20} />
        </button>
      )}

      <div className="hidden min-w-0 flex-1 truncate md:block">{children}</div>

      <div className="min-w-0 flex-1 md:max-w-md md:flex-none md:basis-80 lg:basis-96">
        <QuickSearch />
      </div>

      <div className="flex shrink-0 items-center gap-1 md:ml-auto">
        <Link href="/spaces" className="btn btn-primary" title="Create">
          <Plus size={14} />
          <span className="hidden sm:inline">Create</span>
        </Link>

        <NotificationBell />

        {me && (
          <Menu
            align="right"
            width={240}
            label="Account menu"
            trigger={(open) => (
              <span
                className={`flex h-9 items-center gap-2 rounded-md px-1.5 pr-2 text-sm transition ${
                  open ? "bg-ink-100" : "hover:bg-ink-100"
                }`}
              >
                <Avatar name={label} seed={me.id} size={26} />
                <span className="hidden max-w-[140px] truncate text-ink-800 md:inline">
                  {label}
                </span>
                <ChevronDown size={14} className="text-ink-500" />
              </span>
            )}
          >
            {(close) => (
              <>
                <div className="px-3 py-2">
                  <div className="truncate text-sm font-semibold text-ink-900">
                    {me.display_name || me.email}
                  </div>
                  <div className="truncate text-xs text-ink-500">{me.email}</div>
                </div>
                <MenuDivider />
                <MenuItem
                  onClick={() => {
                    close();
                    router.push("/account");
                  }}
                >
                  <Settings size={14} className="text-ink-500" />
                  Account
                </MenuItem>
                <MenuItem onClick={toggle}>
                  {resolved === "dark" ? (
                    <Sun size={14} className="text-ink-500" />
                  ) : (
                    <Moon size={14} className="text-ink-500" />
                  )}
                  {resolved === "dark" ? "Light theme" : "Dark theme"}
                </MenuItem>
                <MenuDivider />
                <MenuItem
                  onClick={() => {
                    close();
                    void signOut();
                  }}
                >
                  <LogOut size={14} className="text-ink-500" />
                  Sign out
                </MenuItem>
                <MenuDivider />
                <div className="px-3 py-1 text-[11px] text-ink-500">Knowledge {APP_VERSION}</div>
              </>
            )}
          </Menu>
        )}
      </div>
    </header>
  );
}
