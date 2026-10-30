import { cn } from "@/lib/cn";

export type BadgeTone =
  | "neutral"
  | "blue"
  | "green"
  | "amber"
  | "red"
  | "purple"
  | "teal"
  | "magenta";

const tones: Record<BadgeTone, string> = {
  neutral: "bg-ink-100 text-ink-700",
  blue: "bg-brand-50 text-brand-700",
  green: "bg-success-50 text-success-700",
  amber: "bg-warning-50 text-warning-700",
  red: "bg-danger-50 text-danger-700",
  purple: "bg-purple-50 text-purple-700",
  teal: "bg-teal-50 text-teal-600",
  magenta: "bg-magenta-50 text-magenta-600",
};

const tonesBold: Record<BadgeTone, string> = {
  neutral: "bg-ink-700 text-white",
  blue: "bg-brand-600 text-white",
  green: "bg-success-600 text-white",
  amber: "bg-warning-600 text-white",
  red: "bg-danger-600 text-white",
  purple: "bg-purple-600 text-white",
  teal: "bg-teal-600 text-white",
  magenta: "bg-magenta-600 text-white",
};

export function Badge({
  tone = "neutral",
  bold = false,
  children,
  className,
  title,
}: {
  tone?: BadgeTone;
  bold?: boolean;
  children: React.ReactNode;
  className?: string;
  title?: string;
}) {
  return (
    <span
      title={title}
      className={cn(
        "inline-flex items-center gap-1 rounded-xs px-1.5 py-0.5 text-[10.5px] font-semibold uppercase tracking-wide",
        bold ? tonesBold[tone] : tones[tone],
        className
      )}
    >
      {children}
    </span>
  );
}
