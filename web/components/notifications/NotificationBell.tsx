"use client";

import Link from "next/link";
import { Bell } from "lucide-react";

export function NotificationBell() {
  return (
    <Link
      href="/notifications"
      aria-label="Notifications"
      title="Notifications"
      className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md text-ink-700 hover:bg-ink-100 hover:text-ink-900"
    >
      <Bell size={16} />
    </Link>
  );
}
