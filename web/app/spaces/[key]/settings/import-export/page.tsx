"use client";

import { useState } from "react";
import { useParams } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { FileUp } from "lucide-react";
import { ExportSpaceButton } from "@/components/importexport/ExportSpaceButton";
import { ImportDialog } from "@/components/importexport/ImportDialog";
import { getSpace } from "@/lib/api";
import { useMe } from "@/lib/auth";
import { Button } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";
import { Spinner } from "@/components/ui/Spinner";

export default function SpaceImportExportPage() {
  const params = useParams<{ key: string }>();
  const key = params.key;
  const { data: me } = useMe();
  const [importing, setImporting] = useState(false);
  const space = useQuery({
    queryKey: ["importexport", "space", key],
    queryFn: () => getSpace(key),
  });

  const canImport =
    Boolean(me?.is_system_admin) || space.data?.role === "admin" || space.data?.role === "editor";
  const archived = Boolean(space.data?.archived_at);

  return (
    <div className="mx-auto flex w-full max-w-3xl flex-col gap-4 p-4 sm:p-6">
      <h1 className="text-[20px] font-semibold tracking-tight text-ink-900">Import and export</h1>

      {space.isPending ? (
        <div className="flex justify-center py-16">
          <Spinner size={20} />
        </div>
      ) : space.isError ? (
        <EmptyState title="Space unavailable" description={space.error.message} />
      ) : (
        <>
          <section className="surface rounded-lg border border-ink-200 p-4 sm:p-5">
            <h2 className="text-sm font-semibold text-ink-900">Export</h2>
            <p className="mt-1 text-sm text-ink-600">
              Download every page in {key} as a zip of Markdown files, with the page tree
              preserved as folders.
            </p>
            <div className="mt-4">
              <ExportSpaceButton spaceKey={key} />
            </div>
          </section>

          <section className="surface rounded-lg border border-ink-200 p-4 sm:p-5">
            <h2 className="text-sm font-semibold text-ink-900">Import</h2>
            <p className="mt-1 text-sm text-ink-600">
              Create pages from Markdown files or a whole folder. Nested folders become child
              pages.
            </p>
            <div className="mt-4">
              <Button
                variant="primary"
                disabled={!canImport || archived}
                onClick={() => setImporting(true)}
              >
                <FileUp size={14} /> Import Markdown
              </Button>
              {!canImport && (
                <p className="mt-2 text-xs text-ink-500">
                  You need editor access to import pages.
                </p>
              )}
              {canImport && archived && (
                <p className="mt-2 text-xs text-ink-500">
                  This space is archived. Unarchive it to import pages.
                </p>
              )}
            </div>
          </section>
        </>
      )}

      {importing && <ImportDialog spaceKey={key} onClose={() => setImporting(false)} />}
    </div>
  );
}
