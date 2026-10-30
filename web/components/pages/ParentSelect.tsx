"use client";

import { useMemo } from "react";
import type { PageSummary } from "@/lib/api";
import { Select } from "@/components/ui/Select";
import { buildTree, flattenAll } from "@/components/spaces/tree";

export function ParentSelect({
  pages,
  value,
  onChange,
  exclude,
  id,
  disabled,
}: {
  pages: PageSummary[];
  value: string;
  onChange: (id: string) => void;
  exclude?: Set<string>;
  id?: string;
  disabled?: boolean;
}) {
  const options = useMemo(
    () => flattenAll(buildTree(pages)).filter((n) => !exclude?.has(n.page.id)),
    [pages, exclude]
  );
  return (
    <Select
      id={id}
      value={value}
      disabled={disabled}
      onChange={(e) => onChange(e.target.value)}
      className="w-full"
    >
      {options.map((n) => (
        <option key={n.page.id} value={n.page.id}>
          {"  ".repeat(n.depth)}
          {n.page.title || "Untitled"}
          {n.page.status === "draft" ? " (draft)" : ""}
        </option>
      ))}
    </Select>
  );
}
