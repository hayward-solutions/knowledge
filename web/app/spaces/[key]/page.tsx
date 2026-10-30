"use client";

import { useState } from "react";
import { FileText } from "lucide-react";
import { Button } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";
import { NewPageDialog } from "@/components/pages/NewPageDialog";
import { PageBody } from "@/components/pages/PageBody";
import { useSpace } from "@/components/spaces/SpaceContext";

export default function SpaceHomePage() {
  const { space, canEdit } = useSpace();
  const [creating, setCreating] = useState(false);

  if (space.home_page_id) {
    return <PageBody key={space.home_page_id} pageId={space.home_page_id} canonical={false} />;
  }

  return (
    <div className="mx-auto w-full max-w-xl px-4 py-16">
      <EmptyState
        icon={<FileText size={18} />}
        title={`${space.name} has no home page`}
        description={
          canEdit
            ? "Create a page and choose it as the home page in the space settings."
            : "A space admin has not chosen a home page yet."
        }
        action={
          canEdit ? (
            <Button variant="primary" onClick={() => setCreating(true)}>
              Create page
            </Button>
          ) : undefined
        }
      />
      {creating && <NewPageDialog onClose={() => setCreating(false)} />}
    </div>
  );
}
