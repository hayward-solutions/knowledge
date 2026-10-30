"use client";

import { useMemo } from "react";
import { diffWords } from "diff";
import { cn } from "@/lib/cn";

function countWords(s: string): number {
  const m = s.trim().match(/\S+/g);
  return m ? m.length : 0;
}

export function VersionDiff({
  oldText,
  newText,
  className,
}: {
  oldText: string;
  newText: string;
  className?: string;
}) {
  const { parts, added, removed } = useMemo(() => {
    const parts = diffWords(oldText, newText);
    let added = 0;
    let removed = 0;
    for (const p of parts) {
      if (p.added) added += countWords(p.value);
      else if (p.removed) removed += countWords(p.value);
    }
    return { parts, added, removed };
  }, [oldText, newText]);

  const unchanged = added === 0 && removed === 0 && oldText === newText;

  return (
    <div className={className}>
      <div className="mb-3 flex items-center gap-3 text-xs text-ink-600">
        <span className="rounded-xs bg-success-50 px-1.5 py-0.5 font-semibold text-success-700">
          +{added} {added === 1 ? "word" : "words"}
        </span>
        <span className="rounded-xs bg-danger-50 px-1.5 py-0.5 font-semibold text-danger-700">
          -{removed} {removed === 1 ? "word" : "words"}
        </span>
      </div>
      {unchanged ? (
        <p className="rounded-md border border-dashed border-ink-200 px-4 py-6 text-center text-sm text-ink-600">
          No text changes between these versions.
        </p>
      ) : (
        <div
          className={cn(
            "whitespace-pre-wrap break-words rounded-md border border-ink-200 bg-ink-0 p-4 text-[15px] leading-7 text-ink-900"
          )}
        >
          {parts.map((p, i) => {
            if (p.added) {
              return (
                <ins
                  key={i}
                  className="rounded-xs bg-success-100 text-success-700 no-underline"
                  aria-label="Added"
                >
                  {p.value}
                </ins>
              );
            }
            if (p.removed) {
              return (
                <del key={i} className="rounded-xs bg-danger-100 text-danger-700" aria-label="Removed">
                  {p.value}
                </del>
              );
            }
            return <span key={i}>{p.value}</span>;
          })}
        </div>
      )}
    </div>
  );
}
