"use client";

import { useMemo } from "react";
import { diffLines, diffWords, type Change } from "diff";
import { cn } from "@/lib/cn";
import { htmlToStructure, htmlToText } from "./utils";

interface LineView {
  kind: "same" | "added" | "removed" | "changed";
  oldLabel: string;
  newLabel: string;
  oldContent: string;
  newContent: string;
}

function countWords(s: string): number {
  const m = s.trim().match(/\S+/g);
  return m ? m.length : 0;
}

function splitLine(line: string): { label: string; content: string } {
  const i = line.indexOf("|");
  if (i < 0) return { label: "p", content: line };
  return { label: line.slice(0, i).trim(), content: line.slice(i + 1).replace(/^ /, "") };
}

function linesOf(value: string): string[] {
  const parts = value.split("\n");
  if (parts[parts.length - 1] === "") parts.pop();
  return parts;
}

function buildLines(oldStruct: string, newStruct: string): LineView[] {
  const changes: Change[] = diffLines(oldStruct, newStruct);
  const out: LineView[] = [];
  for (let i = 0; i < changes.length; i++) {
    const c = changes[i];
    if (!c.added && !c.removed) {
      linesOf(c.value).forEach((l) => {
        const { label, content } = splitLine(l);
        out.push({ kind: "same", oldLabel: label, newLabel: label, oldContent: content, newContent: content });
      });
      continue;
    }
    const next = changes[i + 1];
    if (c.removed && next && next.added) {
      const removed = linesOf(c.value);
      const added = linesOf(next.value);
      const n = Math.max(removed.length, added.length);
      for (let k = 0; k < n; k++) {
        const r = k < removed.length ? splitLine(removed[k]) : null;
        const a = k < added.length ? splitLine(added[k]) : null;
        if (r && a) {
          out.push({ kind: "changed", oldLabel: r.label, newLabel: a.label, oldContent: r.content, newContent: a.content });
        } else if (r) {
          out.push({ kind: "removed", oldLabel: r.label, newLabel: r.label, oldContent: r.content, newContent: "" });
        } else if (a) {
          out.push({ kind: "added", oldLabel: a.label, newLabel: a.label, oldContent: "", newContent: a.content });
        }
      }
      i++;
      continue;
    }
    linesOf(c.value).forEach((l) => {
      const { label, content } = splitLine(l);
      out.push(
        c.added
          ? { kind: "added", oldLabel: label, newLabel: label, oldContent: "", newContent: content }
          : { kind: "removed", oldLabel: label, newLabel: label, oldContent: content, newContent: "" }
      );
    });
  }
  return out;
}

function Chip({ label, tone }: { label: string; tone?: "added" | "removed" }) {
  return (
    <span
      className={cn(
        "mr-2 inline-block rounded-xs px-1 font-mono text-[11px] leading-5",
        tone === "added" && "bg-success-100 text-success-700",
        tone === "removed" && "bg-danger-100 text-danger-700 line-through",
        !tone && "bg-ink-100 text-ink-600"
      )}
    >
      {label}
    </span>
  );
}

function Words({ oldText, newText }: { oldText: string; newText: string }) {
  const parts = diffWords(oldText, newText);
  return (
    <>
      {parts.map((p, i) => {
        if (p.added) {
          return (
            <ins key={i} className="rounded-xs bg-success-100 text-success-700 no-underline" aria-label="Added">
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
    </>
  );
}

function Line({ line }: { line: LineView }) {
  if (line.kind === "same") {
    return (
      <div className="min-h-[1.75rem]">
        {line.newLabel !== "p" && <Chip label={line.newLabel} />}
        {line.newContent}
      </div>
    );
  }
  if (line.kind === "added") {
    return (
      <div className="min-h-[1.75rem]">
        <Chip label={line.newLabel} tone="added" />
        <ins className="rounded-xs bg-success-100 text-success-700 no-underline" aria-label="Added">
          {line.newContent}
        </ins>
      </div>
    );
  }
  if (line.kind === "removed") {
    return (
      <div className="min-h-[1.75rem]">
        <Chip label={line.oldLabel} tone="removed" />
        <del className="rounded-xs bg-danger-100 text-danger-700" aria-label="Removed">
          {line.oldContent}
        </del>
      </div>
    );
  }
  return (
    <div className="min-h-[1.75rem]">
      {line.oldLabel === line.newLabel ? (
        line.newLabel !== "p" && <Chip label={line.newLabel} />
      ) : (
        <>
          <Chip label={line.oldLabel} tone="removed" />
          <Chip label={line.newLabel} tone="added" />
        </>
      )}
      <Words oldText={line.oldContent} newText={line.newContent} />
    </div>
  );
}

export function VersionDiff({
  oldTitle,
  newTitle,
  oldHtml,
  newHtml,
  className,
}: {
  oldTitle: string;
  newTitle: string;
  oldHtml: string;
  newHtml: string;
  className?: string;
}) {
  const { lines, added, removed, changedLines, textSame, structureSame } = useMemo(() => {
    const oldText = `${oldTitle}\n\n${htmlToText(oldHtml)}`;
    const newText = `${newTitle}\n\n${htmlToText(newHtml)}`;
    let added = 0;
    let removed = 0;
    for (const p of diffWords(oldText, newText)) {
      if (p.added) added += countWords(p.value);
      else if (p.removed) removed += countWords(p.value);
    }
    const oldStruct = `title| ${oldTitle}\n${htmlToStructure(oldHtml)}`;
    const newStruct = `title| ${newTitle}\n${htmlToStructure(newHtml)}`;
    const lines = buildLines(oldStruct, newStruct);
    return {
      lines,
      added,
      removed,
      changedLines: lines.filter((l) => l.kind !== "same").length,
      textSame: oldText === newText,
      structureSame: oldStruct === newStruct,
    };
  }, [oldTitle, newTitle, oldHtml, newHtml]);

  return (
    <div className={className}>
      <div className="mb-3 flex flex-wrap items-center gap-3 text-xs text-ink-600">
        <span className="rounded-xs bg-success-50 px-1.5 py-0.5 font-semibold text-success-700">
          +{added} {added === 1 ? "word" : "words"}
        </span>
        <span className="rounded-xs bg-danger-50 px-1.5 py-0.5 font-semibold text-danger-700">
          -{removed} {removed === 1 ? "word" : "words"}
        </span>
        <span>
          {changedLines} {changedLines === 1 ? "block" : "blocks"} changed
        </span>
      </div>
      {textSame && !structureSame && (
        <div
          role="status"
          className="mb-3 rounded-md border border-brand-100 bg-brand-50 px-3 py-2 text-sm text-brand-700"
        >
          Formatting or structure changed. The text is identical, so the changes below are marks, block types, panels,
          tables or similar.
        </div>
      )}
      {structureSame ? (
        <p className="rounded-md border border-dashed border-ink-200 px-4 py-6 text-center text-sm text-ink-600">
          No changes between these versions.
        </p>
      ) : (
        <div className="break-words rounded-md border border-ink-200 bg-ink-0 p-4 text-[15px] leading-7 text-ink-900">
          {lines.map((l, i) => (
            <Line key={i} line={l} />
          ))}
        </div>
      )}
    </div>
  );
}
