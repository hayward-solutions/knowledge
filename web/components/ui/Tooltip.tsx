import { cn } from "@/lib/cn";

export function Tooltip({
  content,
  side = "top",
  children,
  className,
}: {
  content: React.ReactNode;
  side?: "top" | "bottom";
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <span className={cn("group relative inline-flex", className)}>
      {children}
      <span
        role="tooltip"
        className={cn(
          "pointer-events-none absolute left-1/2 z-50 -translate-x-1/2 whitespace-nowrap rounded-md bg-ink-900 px-2 py-1 text-xs font-medium text-ink-0 opacity-0 shadow-overlay transition-opacity delay-300 group-focus-within:opacity-100 group-hover:opacity-100",
          side === "top" ? "bottom-full mb-1.5" : "top-full mt-1.5"
        )}
      >
        {content}
      </span>
    </span>
  );
}
