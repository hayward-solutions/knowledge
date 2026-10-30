import { Avatar } from "@/components/ui/Avatar";
import { cn } from "@/lib/cn";

export function SpaceIcon({
  space,
  size = 28,
  className,
}: {
  space: { id: string; name: string; icon: string | null };
  size?: number;
  className?: string;
}) {
  if (space.icon) {
    return (
      <span
        aria-hidden
        className={cn(
          "inline-flex shrink-0 items-center justify-center rounded-md bg-ink-100 leading-none",
          className
        )}
        style={{ width: size, height: size, fontSize: Math.round(size * 0.58) }}
      >
        {space.icon}
      </span>
    );
  }
  return <Avatar name={space.name} seed={space.id} size={size} className={cn("rounded-md", className)} />;
}
