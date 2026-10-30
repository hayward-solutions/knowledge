import { cn } from "@/lib/cn";

export function Spinner({
  size = 16,
  label,
  className,
}: {
  size?: number;
  label?: string;
  className?: string;
}) {
  return (
    <span
      role="status"
      aria-label={label ?? "Loading"}
      className={cn(
        "inline-block animate-spin rounded-full border-2 border-brand-200 border-t-brand-600",
        className
      )}
      style={{ width: size, height: size }}
    />
  );
}
