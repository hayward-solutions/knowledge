"use client";

import { SettingsShell } from "@/components/spaces/SettingsShell";
import { useSpace } from "@/components/spaces/SpaceContext";
import { TemplateManager } from "@/components/templates/TemplateManager";

export default function SpaceTemplatesPage() {
  const { space, isEditor } = useSpace();

  return (
    <SettingsShell active="templates" adminOnly={false}>
      <TemplateManager scope={{ kind: "space", key: space.key }} canManage={isEditor} />
    </SettingsShell>
  );
}
