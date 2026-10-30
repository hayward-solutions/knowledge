import Link from "next/link";
import { cn } from "@/lib/cn";

const base =
  "flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm text-ink-800 outline-none hover:bg-ink-100 focus:bg-ink-100";

export function MenuLink({
  href,
  children,
  onClick,
  className,
}: {
  href: string;
  children: React.ReactNode;
  onClick?: () => void;
  className?: string;
}) {
  return (
    <Link href={href} role="menuitem" onClick={onClick} className={cn(base, className)}>
      {children}
    </Link>
  );
}

export function MenuAnchor({
  href,
  children,
  onClick,
  download,
}: {
  href: string;
  children: React.ReactNode;
  onClick?: () => void;
  download?: boolean;
}) {
  return (
    <a href={href} role="menuitem" onClick={onClick} download={download} className={base}>
      {children}
    </a>
  );
}
