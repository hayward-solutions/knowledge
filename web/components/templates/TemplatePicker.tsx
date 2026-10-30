"use client";

import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { listSpaceTemplates, listTemplates, type Template } from "@/lib/api";
import { Select } from "@/components/ui/Select";

export interface TemplatePickerProps {
  spaceKey: string;
  value: string | null;
  onChange: (templateId: string | null) => void;
  disabled?: boolean;
  id?: string;
  className?: string;
}

export function useSpaceTemplateOptions(spaceKey: string) {
  const all = useQuery({
    queryKey: ["templates"],
    queryFn: listTemplates,
    staleTime: 30_000,
  });
  const space = useQuery({
    queryKey: ["templates", "space", spaceKey],
    queryFn: () => listSpaceTemplates(spaceKey),
    staleTime: 30_000,
    enabled: Boolean(spaceKey),
  });

  const { global, local } = useMemo(() => {
    const local = new Map<string, Template>();
    for (const t of space.data ?? []) local.set(t.id, t);
    for (const t of all.data ?? []) {
      if (t.space_id !== null && t.space_key === spaceKey) local.set(t.id, t);
    }
    const global = (all.data ?? []).filter((t) => t.space_id === null);
    return { global, local: Array.from(local.values()) };
  }, [all.data, space.data, spaceKey]);

  return { global, local, isPending: all.isPending && space.isPending };
}

export function TemplatePicker({
  spaceKey,
  value,
  onChange,
  disabled,
  id,
  className,
}: TemplatePickerProps) {
  const { global, local, isPending } = useSpaceTemplateOptions(spaceKey);

  return (
    <Select
      id={id}
      aria-label="Template"
      className={className}
      disabled={disabled || isPending}
      value={value ?? ""}
      onChange={(e) => onChange(e.target.value || null)}
    >
      <option value="">Blank page</option>
      {local.length > 0 && (
        <optgroup label={`${spaceKey} templates`}>
          {local.map((t) => (
            <option key={t.id} value={t.id}>
              {t.name}
            </option>
          ))}
        </optgroup>
      )}
      {global.length > 0 && (
        <optgroup label="Global templates">
          {global.map((t) => (
            <option key={t.id} value={t.id}>
              {t.name}
            </option>
          ))}
        </optgroup>
      )}
    </Select>
  );
}
