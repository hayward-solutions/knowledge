"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { Spinner } from "@/components/ui/Spinner";
import { useSpace } from "@/components/spaces/SpaceContext";

export default function SpaceSettingsIndex() {
  const router = useRouter();
  const { space } = useSpace();
  useEffect(() => {
    router.replace(`/spaces/${space.key}/settings/general`);
  }, [router, space.key]);
  return (
    <div className="flex flex-1 items-center justify-center py-16">
      <Spinner size={20} label="Loading settings" />
    </div>
  );
}
