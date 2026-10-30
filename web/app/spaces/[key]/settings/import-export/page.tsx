"use client";

import { useState } from "react";
import { FileUp } from "lucide-react";
import { ExportSpaceButton } from "@/components/importexport/ExportSpaceButton";
import { ImportDialog } from "@/components/importexport/ImportDialog";
import { SettingsShell } from "@/components/spaces/SettingsShell";
import { useSpace } from "@/components/spaces/SpaceContext";
import { Button } from "@/components/ui/Button";

export default function SpaceImportExportPage() {
  const { space, isEditor, archived } = useSpace();
  const key = space.key;
  const [importing, setImporting] = useState(false);
  const canImport = isEditor;

  return (
    <SettingsShell active="import-export" adminOnly={false}>
      <div className="flex flex-col gap-4">
        <section className="surface rounded-lg border border-ink-200 p-4 sm:p-5">
          <h2 className="text-sm font-semibold text-ink-900">Export</h2>
          <p className="mt-1 text-sm text-ink-600">
            Download every page in {key} as a zip of Markdown files, with the page tree preserved
            as folders.
          </p>
          <div className="mt-4">
            <ExportSpaceButton spaceKey={key} />
          </div>
        </section>

        <section className="surface rounded-lg border border-ink-200 p-4 sm:p-5">
          <h2 className="text-sm font-semibold text-ink-900">Import</h2>
          <p className="mt-1 text-sm text-ink-600">
            Create pages from Markdown files or a whole folder. Nested folders become child pages.
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
              <p className="mt-2 text-xs text-ink-500">You need editor access to import pages.</p>
            )}
            {canImport && archived && (
              <p className="mt-2 text-xs text-ink-500">
                This space is archived. Unarchive it to import pages.
              </p>
            )}
          </div>
        </section>
      </div>

      {importing && <ImportDialog spaceKey={key} onClose={() => setImporting(false)} />}
    </SettingsShell>
  );
}
