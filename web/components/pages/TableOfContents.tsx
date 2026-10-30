"use client";

import { useEffect, useState } from "react";
import { cn } from "@/lib/cn";
import type { HeadingInfo } from "@/lib/content/headings";

export function TableOfContents({ headings }: { headings: HeadingInfo[] }) {
  const [active, setActive] = useState<string | null>(null);

  useEffect(() => {
    if (headings.length === 0) return;
    const visible = new Map<string, boolean>();
    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) visible.set(entry.target.id, entry.isIntersecting);
        const first = headings.find((h) => visible.get(h.id));
        if (first) setActive(first.id);
      },
      { rootMargin: "-72px 0px -65% 0px" }
    );
    const timer = window.setTimeout(() => {
      headings.forEach((h) => {
        const el = document.getElementById(h.id);
        if (el) observer.observe(el);
      });
    }, 100);
    return () => {
      window.clearTimeout(timer);
      observer.disconnect();
    };
  }, [headings]);

  if (headings.length === 0) return null;
  const min = Math.min(...headings.map((h) => h.level));

  return (
    <nav aria-label="On this page" className="text-sm">
      <div className="mb-2 text-[11px] font-semibold uppercase tracking-wide text-ink-500">On this page</div>
      <ul className="space-y-0.5 border-l border-ink-200">
        {headings.map((h, i) => (
          <li key={`${h.id}-${i}`}>
            <a
              href={`#${h.id}`}
              onClick={(e) => {
                const el = document.getElementById(h.id);
                if (!el) return;
                e.preventDefault();
                el.scrollIntoView({ behavior: "smooth", block: "start" });
                window.history.replaceState(null, "", `#${h.id}`);
                setActive(h.id);
              }}
              style={{ paddingLeft: 10 + Math.min(h.level - min, 3) * 12 }}
              className={cn(
                "-ml-px block border-l-2 py-1 pr-2 leading-snug transition",
                active === h.id
                  ? "border-brand-600 font-medium text-brand-700"
                  : "border-transparent text-ink-600 hover:text-ink-900"
              )}
            >
              {h.text}
            </a>
          </li>
        ))}
      </ul>
    </nav>
  );
}
