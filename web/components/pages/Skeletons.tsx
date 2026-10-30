import { cn } from "@/lib/cn";

export function Skeleton({ className }: { className?: string }) {
  return <div aria-hidden className={cn("animate-pulse rounded bg-ink-100", className)} />;
}

export function PageSkeleton() {
  return (
    <div className="mx-auto w-full max-w-[860px] space-y-4 px-4 py-8 sm:px-8" aria-busy="true">
      <Skeleton className="h-4 w-48" />
      <Skeleton className="h-9 w-2/3" />
      <Skeleton className="h-4 w-80" />
      <div className="space-y-2 pt-6">
        <Skeleton className="h-4 w-full" />
        <Skeleton className="h-4 w-full" />
        <Skeleton className="h-4 w-5/6" />
        <Skeleton className="h-4 w-full" />
        <Skeleton className="h-4 w-3/4" />
      </div>
    </div>
  );
}

export function ListSkeleton({ rows = 5 }: { rows?: number }) {
  return (
    <div className="space-y-2" aria-busy="true">
      {Array.from({ length: rows }).map((_, i) => (
        <Skeleton key={i} className="h-10 w-full" />
      ))}
    </div>
  );
}
