"use client";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useState } from "react";
import { ThemeProvider } from "@/lib/theme";
import { ToastProvider } from "@/components/ui/Toast";
import { NotificationsProvider } from "@/components/notifications/NotificationsProvider";

export function Providers({ children }: { children: React.ReactNode }) {
  const [qc] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: { staleTime: 10_000, refetchOnWindowFocus: false },
        },
      })
  );
  return (
    <ThemeProvider>
      <QueryClientProvider client={qc}>
        <ToastProvider>
          <NotificationsProvider>{children}</NotificationsProvider>
        </ToastProvider>
      </QueryClientProvider>
    </ThemeProvider>
  );
}
