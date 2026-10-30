"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Eye, EyeOff } from "lucide-react";
import {
  getWatchState,
  unwatchTarget,
  watchTarget,
  type WatchState,
  type WatchTargetType,
} from "@/lib/api";
import { cn } from "@/lib/cn";
import { Tooltip } from "@/components/ui/Tooltip";
import { useToast } from "@/components/ui/Toast";

export interface WatchToggleProps {
  type: WatchTargetType;
  id: string;
  initial?: boolean;
  size?: "sm" | "md";
  showLabel?: boolean;
  className?: string;
}

export function WatchToggle({
  type,
  id,
  initial,
  size = "md",
  showLabel,
  className,
}: WatchToggleProps) {
  const qc = useQueryClient();
  const toast = useToast();
  const key = ["watch", type, id] as const;
  const labelled = showLabel ?? type === "space";

  const state = useQuery<WatchState>({
    queryKey: key,
    queryFn: () => getWatchState(type, id),
    initialData: initial === undefined ? undefined : { watching: initial },
    staleTime: 30_000,
    enabled: Boolean(id),
  });

  const watching = state.data?.watching ?? false;

  const toggle = useMutation({
    mutationFn: async (next: boolean) => {
      if (next) await watchTarget(type, id);
      else await unwatchTarget(type, id);
      return next;
    },
    onMutate: async (next) => {
      await qc.cancelQueries({ queryKey: key });
      const prev = qc.getQueryData<WatchState>(key);
      qc.setQueryData<WatchState>(key, { watching: next });
      return { prev };
    },
    onError: (e: Error, _next, ctx) => {
      if (ctx?.prev) qc.setQueryData(key, ctx.prev);
      toast.error(e.message);
    },
    onSuccess: (next) => {
      toast.success(
        next
          ? `You are now watching this ${type}`
          : `You stopped watching this ${type}`
      );
    },
    onSettled: () => {
      qc.invalidateQueries({ queryKey: key });
    },
  });

  const label = watching ? (labelled ? "Watching" : "Stop watching") : labelled ? `Watch ${type}` : "Watch";
  const tip = watching ? `Stop watching this ${type}` : `Get notified about changes to this ${type}`;
  const Icon = watching ? Eye : EyeOff;

  return (
    <Tooltip content={tip} side="bottom">
      <button
        type="button"
        aria-pressed={watching}
        aria-label={labelled ? undefined : label}
        disabled={state.isPending || toggle.isPending}
        onClick={() => toggle.mutate(!watching)}
        className={cn(
          "btn",
          watching ? "btn-secondary" : "btn-ghost",
          size === "sm" && "btn-sm",
          !labelled && (size === "sm" ? "!w-[26px] !px-0" : "!w-8 !px-0"),
          watching && "text-brand-700",
          className
        )}
      >
        <Icon size={size === "sm" ? 14 : 16} />
        {labelled && <span>{label}</span>}
      </button>
    </Tooltip>
  );
}
